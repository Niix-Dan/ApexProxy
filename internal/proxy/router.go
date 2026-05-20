package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
	"github.com/niix-dan/apexproxy/internal/metrics"
)

type RuntimeTarget struct {
	URL   string
	Proxy *httputil.ReverseProxy
}

type RuntimeRoute struct {
	Config           config.Route
	Targets          []*RuntimeTarget
	FlattenedTargets []*RuntimeTarget
	Counter          uint64
}

type Router struct {
	Routes []*RuntimeRoute
}

func NewRouter(cfg *config.Config) *Router {
	var rRoutes []*RuntimeRoute
	for _, r := range cfg.Routing {
		var rTargets []*RuntimeTarget
		var flattened []*RuntimeTarget

		for _, t := range r.Targets {
			u, _ := url.Parse(t.URL)
			rt := &RuntimeTarget{
				URL:   t.URL,
				Proxy: httputil.NewSingleHostReverseProxy(u),
			}

			rt.Proxy.ModifyResponse = func(resp *http.Response) error {
				resp.Header.Del("Server")
				resp.Header.Del("X-Powered-By")
				resp.Header.Del("X-AspNet-Version")
				resp.Header.Del("X-AspNetMvc-Version")

				resp.Header.Set("X-Content-Type-Options", "nosniff")
				resp.Header.Set("X-Frame-Options", "SAMEORIGIN")
				resp.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
				resp.Header.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")

				return nil
			}

			rTargets = append(rTargets, rt)

			weight := t.Weight
			if weight <= 0 {
				weight = 1
			}
			for i := 0; i < weight; i++ {
				flattened = append(flattened, rt)
			}
		}

		rRoutes = append(rRoutes, &RuntimeRoute{
			Config:           r,
			Targets:          rTargets,
			FlattenedTargets: flattened,
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

	reqBodyCounter := &bodyReadCounter{ReadCloser: req.Body}
	if req.Body != nil {
		req.Body = reqBodyCounter
	}

	interceptor := &responseWriterInterceptor{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	matched := r.matchRoute(req)
	if matched == nil {
		http.Error(interceptor, "Bad Gateway", http.StatusBadGateway)
		r.recordMetrics(interceptor, req, reqBodyCounter, startTime, "none", "none")
		return
	}

	target := matched.chooseTarget(req.RemoteAddr)
	if target == nil {
		http.Error(interceptor, "Service Unavailable", http.StatusServiceUnavailable)
		r.recordMetrics(interceptor, req, reqBodyCounter, startTime, "none", matched.Config.Strategy)
		return
	}

	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		ip = req.RemoteAddr
	}

	req.Header.Set("X-Real-IP", ip)
	req.Header.Set("X-Forwarded-Host", req.Host)

	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	target.Proxy.ServeHTTP(interceptor, req)

	r.recordMetrics(interceptor, req, reqBodyCounter, startTime, target.URL, matched.Config.Strategy)
}

func (r *Router) matchRoute(req *http.Request) *RuntimeRoute {
	for _, rr := range r.Routes {
		hostMatch := rr.Config.Host == "" || rr.Config.Host == req.Host
		if !hostMatch && strings.HasPrefix(rr.Config.Host, "*.") {
			hostMatch = strings.HasSuffix(req.Host, rr.Config.Host[1:])
		}

		pathMatch := rr.Config.Path == "" || strings.HasPrefix(req.URL.Path, rr.Config.Path)

		if hostMatch && pathMatch {
			return rr
		}
	}
	return nil
}

func (r *Router) recordMetrics(w *responseWriterInterceptor, req *http.Request, body *bodyReadCounter, start time.Time, target, strategy string) {
	metrics.Instance.RecordRequest(
		w.statusCode, req.Method, req.URL.Path, time.Since(start),
		req.RemoteAddr, target, strategy, body.bytesRead, w.bytesWritten,
	)
}
