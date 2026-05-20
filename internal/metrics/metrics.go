package metrics

import (
	"crypto/hmac"
	"crypto/sha256"

	"encoding/hex"
	"encoding/json"

	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const InternalSecret = "apex-internal-metrics-v1"

func SignRequest(req *http.Request) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(InternalSecret))
	mac.Write([]byte(ts))
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Apex-Timestamp", ts)
	req.Header.Set("X-Apex-Signature", sig)
}

func verifyRequest(req *http.Request) bool {
	ts := req.Header.Get("X-Apex-Timestamp")
	sig := req.Header.Get("X-Apex-Signature")
	if ts == "" || sig == "" {
		return false
	}
	var tsInt int64
	if _, err := fmt.Sscan(ts, &tsInt); err != nil {
		return false
	}
	diff := time.Now().Unix() - tsInt
	if diff > 5 || diff < -5 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(InternalSecret))
	mac.Write([]byte(ts))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Latency   string `json:"latency"`
	IP        string `json:"ip"`
}

type RouteStat struct {
	Path     string `json:"path"`
	Strategy string `json:"strategy"`
	Target   string `json:"target"`
	Health   string `json:"health"`
	Reqs     int    `json:"reqs"`
	Errors   int    `json:"errors"`
}

type ProxyStats struct {
	Uptime            string      `json:"uptime"`
	ReqsPerSec        float64     `json:"reqs_per_sec"`
	ActiveConnections int64       `json:"active_connections"`
	AvgLatency        string      `json:"avg_latency"`
	MemUsage          string      `json:"mem_usage"`
	BandwidthIn       string      `json:"bandwidth_in"`
	BandwidthOut      string      `json:"bandwidth_out"`
	Codes2xx          int64       `json:"codes_2xx"`
	Codes3xx          int64       `json:"codes_3xx"`
	Codes4xx          int64       `json:"codes_4xx"`
	Codes5xx          int64       `json:"codes_5xx"`
	Routes            []RouteStat `json:"routes"`
	RecentLogs        []LogEntry  `json:"recent_logs"`
}

type Registry struct {
	mu           sync.RWMutex
	StartTime    time.Time
	activeConns  atomic.Int64
	codes2xx     atomic.Int64
	codes3xx     atomic.Int64
	codes4xx     atomic.Int64
	codes5xx     atomic.Int64
	bytesIn      atomic.Int64
	bytesOut     atomic.Int64
	latencySum   atomic.Int64
	latencyCount atomic.Int64
	reqWindowMu  sync.Mutex
	reqWindow    []time.Time
	routes       []RouteStat
	recentLogs   []LogEntry
}

var Instance = &Registry{
	StartTime: time.Now(),
}

func (r *Registry) RecordRequest(
	status int,
	method, path string,
	latency time.Duration,
	ip, target, strategy string,
	bytesIn, bytesOut int64,
) {
	switch {
	case status >= 200 && status < 300:
		r.codes2xx.Add(1)
	case status >= 300 && status < 400:
		r.codes3xx.Add(1)
	case status >= 400 && status < 500:
		r.codes4xx.Add(1)
	case status >= 500:
		r.codes5xx.Add(1)
	}

	r.latencySum.Add(latency.Nanoseconds())
	r.latencyCount.Add(1)
	r.bytesIn.Add(bytesIn)
	r.bytesOut.Add(bytesOut)

	now := time.Now()
	r.reqWindowMu.Lock()
	r.reqWindow = append(r.reqWindow, now)
	cutoff := now.Add(-10 * time.Second)
	i := 0
	for i < len(r.reqWindow) && r.reqWindow[i].Before(cutoff) {
		i++
	}
	r.reqWindow = r.reqWindow[i:]
	r.reqWindowMu.Unlock()

	health := "UP"
	if status == http.StatusBadGateway || status == http.StatusServiceUnavailable {
		health = "DOWN"
	}

	entry := LogEntry{
		Timestamp: now.Format("15:04:05"),
		Status:    status,
		Method:    method,
		Path:      path,
		Latency:   latency.Round(time.Microsecond).String(),
		IP:        ip,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.recentLogs = append([]LogEntry{entry}, r.recentLogs...)
	if len(r.recentLogs) > 50 {
		r.recentLogs = r.recentLogs[:50]
	}

	found := false
	for i, route := range r.routes {
		if route.Path == path && route.Target == target {
			r.routes[i].Reqs++
			r.routes[i].Health = health
			if status >= 500 {
				r.routes[i].Errors++
			}
			found = true
			break
		}
	}
	if !found {
		errs := 0
		if status >= 500 {
			errs = 1
		}
		r.routes = append(r.routes, RouteStat{
			Path:     path,
			Strategy: strategy,
			Target:   target,
			Health:   health,
			Reqs:     1,
			Errors:   errs,
		})
	}
}

func (r *Registry) ConnOpen()  { r.activeConns.Add(1) }
func (r *Registry) ConnClose() { r.activeConns.Add(-1) }

func (r *Registry) Reset() {
	r.codes2xx.Store(0)
	r.codes3xx.Store(0)
	r.codes4xx.Store(0)
	r.codes5xx.Store(0)
	r.latencySum.Store(0)
	r.latencyCount.Store(0)
	r.bytesIn.Store(0)
	r.bytesOut.Store(0)

	r.reqWindowMu.Lock()
	r.reqWindow = r.reqWindow[:0]
	r.reqWindowMu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.routes {
		r.routes[i].Reqs = 0
		r.routes[i].Errors = 0
		r.routes[i].Health = "UP"
	}
	r.recentLogs = r.recentLogs[:0]
}

func (r *Registry) snapshot() ProxyStats {
	uptime := time.Since(r.StartTime).Round(time.Second).String()

	r.reqWindowMu.Lock()
	windowLen := len(r.reqWindow)
	r.reqWindowMu.Unlock()
	reqsPerSec := float64(windowLen) / 10.0

	avgLatency := "—"
	if cnt := r.latencyCount.Load(); cnt > 0 {
		avg := time.Duration(r.latencySum.Load() / cnt)
		avgLatency = avg.Round(time.Microsecond).String()
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memUsage := formatBytes(int64(ms.Alloc))

	uptimeSec := time.Since(r.StartTime).Seconds()
	if uptimeSec < 1 {
		uptimeSec = 1
	}
	bwIn := formatBytesPerSec(float64(r.bytesIn.Load()) / uptimeSec)
	bwOut := formatBytesPerSec(float64(r.bytesOut.Load()) / uptimeSec)

	r.mu.RLock()
	routes := make([]RouteStat, len(r.routes))
	copy(routes, r.routes)
	logs := make([]LogEntry, len(r.recentLogs))
	copy(logs, r.recentLogs)
	r.mu.RUnlock()

	if len(logs) > 3 {
		logs = logs[:3]
	}

	return ProxyStats{
		Uptime:            uptime,
		ReqsPerSec:        reqsPerSec,
		ActiveConnections: r.activeConns.Load(),
		AvgLatency:        avgLatency,
		MemUsage:          memUsage,
		BandwidthIn:       bwIn,
		BandwidthOut:      bwOut,
		Codes2xx:          r.codes2xx.Load(),
		Codes3xx:          r.codes3xx.Load(),
		Codes4xx:          r.codes4xx.Load(),
		Codes5xx:          r.codes5xx.Load(),
		Routes:            routes,
		RecentLogs:        logs,
	}
}

func HTTPHandler(w http.ResponseWriter, req *http.Request) {
	if !isLocalhost(req) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !verifyRequest(req) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	stats := Instance.snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(stats)
}

func ResetHandler(w http.ResponseWriter, req *http.Request) {
	if !isLocalhost(req) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !verifyRequest(req) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	Instance.Reset()
	w.WriteHeader(http.StatusNoContent)
}

func isLocalhost(req *http.Request) bool {
	host := req.RemoteAddr
	for _, local := range []string{"127.0.0.1:", "[::1]:"} {
		if len(host) >= len(local) && host[:len(local)] == local {
			return true
		}
	}
	return false
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatBytesPerSec(bps float64) string {
	switch {
	case bps >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", bps/(1<<20))
	case bps >= 1<<10:
		return fmt.Sprintf("%.1f KB/s", bps/(1<<10))
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}
