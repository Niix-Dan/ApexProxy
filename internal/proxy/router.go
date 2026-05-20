package proxy

import (
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
	"github.com/niix-dan/apexproxy/internal/metrics"
)

type bodyReadCounter struct {
	io.ReadCloser
	bytesRead int64
}

func (b *bodyReadCounter) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

type RuntimeRoute struct {
	Config        config.Route
	FlattenedURLs []string
	Counter       uint64
}

type Router struct {
	Routes []*RuntimeRoute
}

func NewRouter(cfg *config.Config) *Router {
	var rRoutes []*RuntimeRoute
	for _, r := range cfg.Routing {
		var flattened []string
		for _, t := range r.Targets {
			weight := t.Weight
			if weight <= 0 {
				weight = 1
			}
			for i := 0; i < weight; i++ {
				flattened = append(flattened, t.URL)
			}
		}
		rRoutes = append(rRoutes, &RuntimeRoute{
			Config:        r,
			FlattenedURLs: flattened,
		})
	}
	sort.Slice(rRoutes, func(i, j int) bool {
		return rRoutes[i].Config.Priority > rRoutes[j].Config.Priority
	})
	return &Router{Routes: rRoutes}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	metrics.Instance.ConnOpen()
	defer metrics.Instance.ConnClose()

	startTime := time.Now()

	// bytesin
	reqBodyCounter := &bodyReadCounter{ReadCloser: req.Body}
	if req.Body != nil {
		req.Body = reqBodyCounter
	}

	// bytesout
	interceptor := &responseWriterInterceptor{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	var matched *RuntimeRoute
	for _, rr := range r.Routes {
		hostMatch := false
		if rr.Config.Host == "" {
			hostMatch = true
		} else if strings.HasPrefix(rr.Config.Host, "*.") {
			suffix := rr.Config.Host[1:]
			hostMatch = strings.HasSuffix(req.Host, suffix)
		} else {
			hostMatch = rr.Config.Host == req.Host
		}
		pathMatch := rr.Config.Path == "" || strings.HasPrefix(req.URL.Path, rr.Config.Path)
		if hostMatch && pathMatch {
			matched = rr
			break
		}
	}

	if matched == nil {
		http.Error(interceptor, "Bad Gateway", http.StatusBadGateway)
		metrics.Instance.RecordRequest(
			http.StatusBadGateway, req.Method, req.URL.Path, time.Since(startTime),
			req.RemoteAddr, "none", "none", reqBodyCounter.bytesRead, interceptor.bytesWritten,
		)
		return
	}

	var targetRawURL string
	strategy := matched.Config.Strategy

	switch strategy {
	case "round-robin":
		if len(matched.FlattenedURLs) == 0 {
			http.Error(interceptor, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		idx := atomic.AddUint64(&matched.Counter, 1) - 1
		targetRawURL = matched.FlattenedURLs[idx%uint64(len(matched.FlattenedURLs))]
	case "ip-hash":
		if len(matched.Config.Targets) == 0 {
			http.Error(interceptor, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			ip = req.RemoteAddr
		}
		h := fnv.New32a()
		h.Write([]byte(ip))
		idx := h.Sum32() % uint32(len(matched.Config.Targets))
		targetRawURL = matched.Config.Targets[idx].URL
	case "single", "":
		if len(matched.Config.Targets) > 0 {
			targetRawURL = matched.Config.Targets[0].URL
		}
	default:
		if len(matched.Config.Targets) > 0 {
			targetRawURL = matched.Config.Targets[0].URL
		}
	}

	if targetRawURL == "" {
		http.Error(interceptor, "Service Unavailable", http.StatusServiceUnavailable)
		metrics.Instance.RecordRequest(
			http.StatusServiceUnavailable, req.Method, req.URL.Path, time.Since(startTime),
			req.RemoteAddr, "none", strategy, reqBodyCounter.bytesRead, interceptor.bytesWritten,
		)
		return
	}

	targetURL, err := url.Parse(targetRawURL)
	if err != nil {
		http.Error(interceptor, "Internal Error", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(interceptor, req)

	metrics.Instance.RecordRequest(
		interceptor.statusCode,
		req.Method,
		req.URL.Path,
		time.Since(startTime),
		req.RemoteAddr,
		targetRawURL,
		strategy,
		reqBodyCounter.bytesRead,
		interceptor.bytesWritten,
	)
}
