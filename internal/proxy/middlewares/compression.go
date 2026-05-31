package middlewares

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/niix-dan/apexproxy/internal/config"
)

type Compressor struct {
	types []string
	pool  sync.Pool
}

func NewCompressor(cfg *config.CompressionConfig) *Compressor {
	level := cfg.Level
	if level < gzip.BestSpeed || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}

	c := &Compressor{types: cfg.Types}
	c.pool = sync.Pool{
		New: func() any {
			gz, _ := gzip.NewWriterLevel(io.Discard, level)
			return gz
		},
	}
	return c
}

func (c *Compressor) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		r = r.Clone(r.Context())
		r.Header.Del("Accept-Encoding")

		gw := &gzipWriter{
			ResponseWriter: w,
			c:              c,
			statusCode:     http.StatusOK,
		}

		defer gw.finish()

		//w.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(gw, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	c          *Compressor
	gz         *gzip.Writer
	statusCode int
	decided    bool
	skip       bool
}

func (gw *gzipWriter) WriteHeader(statusCode int) {
	gw.statusCode = statusCode
	if !gw.decided {
		gw.decide()
	}
}

func (gw *gzipWriter) Write(b []byte) (int, error) {
	if !gw.decided {
		gw.decide()
	}
	if gw.skip {
		return gw.ResponseWriter.Write(b)
	}
	return gw.gz.Write(b)
}

func (gw *gzipWriter) Header() http.Header {
	return gw.ResponseWriter.Header()
}

func (gw *gzipWriter) Flush() {
	if gw.gz != nil {
		gw.gz.Flush()
	}
	if f, ok := gw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (gw *gzipWriter) Unwrap() http.ResponseWriter {
	return gw.ResponseWriter
}

func (gw *gzipWriter) decide() {
	gw.decided = true
	h := gw.ResponseWriter.Header()

	eligible := gw.statusHasBody() &&
		h.Get("Content-Encoding") == "" &&
		gw.c.shouldCompress(h.Get("Content-Type"))

	if !eligible {
		gw.skip = true
		gw.ResponseWriter.WriteHeader(gw.statusCode)
		return
	}

	gz := gw.c.pool.Get().(*gzip.Writer)
	gz.Reset(gw.ResponseWriter)
	gw.gz = gz

	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")

	if v := h.Get("Vary"); !strings.Contains(strings.ToLower(v), "accept-encoding") {
		if v == "" {
			h.Set("Vary", "Accept-Encoding")
		} else {
			h.Set("Vary", v+", Accept-Encoding")
		}
	}

	gw.ResponseWriter.WriteHeader(gw.statusCode)
}

func (gw *gzipWriter) finish() {
	if !gw.decided {
		gw.decide()
	}

	if gw.gz != nil {
		gw.gz.Close()
		gw.c.pool.Put(gw.gz)
		gw.gz = nil
	}
}

func (gw *gzipWriter) statusHasBody() bool {
	s := gw.statusCode
	return s != http.StatusNoContent &&
		s != http.StatusNotModified &&
		!(s >= 100 && s < 200)
}

func (c *Compressor) shouldCompress(contentType string) bool {
	if len(c.types) == 0 {
		return true
	}

	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	for _, allowed := range c.types {
		if strings.ToLower(strings.TrimSpace(allowed)) == ct {
			return true
		}
	}
	return false
}
