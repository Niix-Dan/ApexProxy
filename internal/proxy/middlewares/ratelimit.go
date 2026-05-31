package middlewares

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*rateBucket
	rpm     int
	done    chan struct{}
}

type rateBucket struct {
	mu sync.Mutex
	ts []time.Time
}

func NewRateLimiter(rpm int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*rateBucket),
		rpm:     rpm,
		done:    make(chan struct{}),
	}
	go rl.sweepLoop()
	return rl
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.RLock()
	b, ok := rl.buckets[ip]
	rl.mu.RUnlock()

	if !ok {
		rl.mu.Lock()
		b, ok = rl.buckets[ip]
		if !ok {
			b = &rateBucket{}
			rl.buckets[ip] = b
		}
		rl.mu.Unlock()
	}

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	b.mu.Lock()
	defer b.mu.Unlock()

	i := 0
	for i < len(b.ts) && b.ts[i].Before(cutoff) {
		i++
	}
	b.ts = b.ts[i:]

	if len(b.ts) >= rl.rpm {
		return false
	}

	b.ts = append(b.ts, now)
	return true
}

func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)

		if err != nil {
			ip = r.RemoteAddr
		}

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) Stop() { close(rl.done) }

func (rl *RateLimiter) sweepLoop() {
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			rl.sweep()
		case <-rl.done:
			return
		}
	}
}

func (rl *RateLimiter) sweep() {
	cutoff := time.Now().Add(-time.Minute)
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, b := range rl.buckets {
		b.mu.Lock()
		i := 0
		for i < len(b.ts) && b.ts[i].Before(cutoff) {
			i++
		}
		b.ts = b.ts[i:]
		if len(b.ts) == 0 {
			delete(rl.buckets, ip)
		}
		b.mu.Unlock()
	}
}
