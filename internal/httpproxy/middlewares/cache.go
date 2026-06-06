package middlewares

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
)

const defaultMaxBodyBytes = 1 << 20 // 1 MB

type cacheEntry struct {
	status    int
	headers   http.Header
	body      []byte
	expiresAt time.Time
}

type ResponseCache struct {
	mu           sync.RWMutex
	entries      map[string]*cacheEntry
	maxEntries   int
	ttl          time.Duration
	maxBodyBytes int64
	done         chan struct{}
}

func NewResponseCache(cfg config.CacheConfig) *ResponseCache {
	ttl := time.Duration(cfg.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	maxBody := int64(cfg.MaxBodyBytes)
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}
	maxEnt := cfg.MaxEntries
	if maxEnt <= 0 {
		maxEnt = 1000
	}

	c := &ResponseCache{
		entries:      make(map[string]*cacheEntry),
		maxEntries:   maxEnt,
		ttl:          ttl,
		maxBodyBytes: maxBody,
		done:         make(chan struct{}),
	}

	go c.evictLoop()
	return c
}

func (c *ResponseCache) Stop() { close(c.done) }

func (c *ResponseCache) cacheKey(r *http.Request) string { return r.Host + "|" + r.URL.RequestURI() }

func (c *ResponseCache) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestCacheable(r) {
			next.ServeHTTP(w, r)
			return
		}

		key := c.cacheKey(r)

		c.mu.RLock()
		entry, hit := c.entries[key]
		c.mu.RUnlock()

		if hit && time.Now().Before(entry.expiresAt) {
			h := w.Header()
			for k, vals := range entry.headers {
				h[k] = vals
			}
			h.Set("X-Cache", "HIT")
			w.WriteHeader(entry.status)
			w.Write(entry.body) // nolint:errcheck
			return
		}

		cr := &capturedResponse{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			status:         http.StatusOK,
			maxBody:        c.maxBodyBytes,
		}
		next.ServeHTTP(cr, r)

		if cr.overflow || !responseCacheable(cr) {
			return
		}

		c.mu.Lock()
		if len(c.entries) < c.maxEntries {
			bodyCopy := append([]byte(nil), cr.body.Bytes()...)
			h := make(http.Header, len(cr.captured))
			for k, v := range cr.captured {
				h[k] = v
			}
			c.entries[key] = &cacheEntry{
				status:    cr.status,
				headers:   h,
				body:      bodyCopy,
				expiresAt: time.Now().Add(c.ttl),
			}
		}
		c.mu.Unlock()
	})
}

func requestCacheable(cr *http.Request) bool {
	if cr.Method != http.MethodGet {
		return false
	}

	cc := cr.Header.Get("Cache-Control")
	return !strings.Contains(cc, "no-cache") && !strings.Contains(cc, "no-store")
}

func responseCacheable(cr *capturedResponse) bool {
	if cr.status < 200 || cr.status >= 300 {
		return false
	}
	if cr.captured == nil {
		return false
	}

	cc := cr.captured.Get("Cache-Control")
	return !strings.Contains(cc, "no-store") && !strings.Contains(cc, "private")
}

func (c *ResponseCache) evictLoop() {
	t := time.NewTicker(c.ttl)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := time.Now()
			c.mu.Lock()
			for k, e := range c.entries {
				if now.After(e.expiresAt) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}

// CapturedResponse
type capturedResponse struct {
	http.ResponseWriter
	captured http.Header
	body     *bytes.Buffer
	status   int
	maxBody  int64
	written  int64
	overflow bool
	sent     bool
}

func (cr *capturedResponse) Header() http.Header {
	if cr.captured == nil {
		cr.captured = make(http.Header)
	}
	return cr.captured
}

func (cr *capturedResponse) WriteHeader(code int) {
	cr.status = code
	if cr.sent {
		return
	}
	cr.sent = true
	h := cr.ResponseWriter.Header()
	for k, vals := range cr.captured {
		h[k] = vals
	}
	cr.ResponseWriter.WriteHeader(code)
}

func (cr *capturedResponse) Write(b []byte) (int, error) {
	if !cr.sent {
		cr.WriteHeader(http.StatusOK)
	}
	if !cr.overflow {
		cr.written += int64(len(b))
		if cr.written > cr.maxBody {
			cr.overflow = true
			cr.body = nil
		} else {
			cr.body.Write(b) //nolint:errcheck
		}
	}
	return cr.ResponseWriter.Write(b)
}

func (cr *capturedResponse) Flush() {
	if f, ok := cr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
