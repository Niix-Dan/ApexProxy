package httpproxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
	"github.com/niix-dan/apexproxy/internal/httpproxy/middlewares"
)

type Server struct {
	cfg     *config.Config
	handler http.Handler
	//router     *Router
	tlsManager *TLSManager
}

func NewServer(cfg *config.Config) (*Server, error) {
	tlsManager, err := NewTLSManager(cfg)
	if err != nil {
		return nil, err
	}

	var handler http.Handler = NewRouter(cfg)

	ipCfg := cfg.Proxy.IPFilter
	if len(ipCfg.BlacklistCIDRs) > 0 || len(ipCfg.WhitelistCIDRs) > 0 {
		ipf, err := middlewares.NewIPFilter(ipCfg.BlacklistCIDRs, ipCfg.WhitelistCIDRs)
		if err != nil {
			return nil, fmt.Errorf("ip filter: %w", err)
		}
		handler = ipf.Handler(handler)
	}

	if cfg.Proxy.RateLimit.Enabled && cfg.Proxy.RateLimit.RequestsPerMinute > 0 {
		handler = middlewares.NewRateLimiter(cfg.Proxy.RateLimit.RequestsPerMinute).Handler(handler)
	}

	if cfg.Proxy.Compression.Enabled {
		handler = middlewares.NewCompressor(&cfg.Proxy.Compression).Handler(handler)
	}

	if cfg.Proxy.Cache.Enabled {
		handler = middlewares.NewResponseCache(cfg.Proxy.Cache).Handler(handler)
	}

	return &Server{
		cfg:        cfg,
		handler:    handler,
		tlsManager: tlsManager,
	}, nil
}

func (s *Server) Start() error {
	errChan := make(chan error, 2)
	started := 0
	httpsPort := s.cfg.Server.HTTPSPort

	if s.cfg.Server.HTTPPort > 0 {
		started++
		go func() {
			addr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)
			redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host, _, err := net.SplitHostPort(r.Host)
				if err != nil {
					host = r.Host
				}
				var target string
				if httpsPort == 443 {
					target = fmt.Sprintf("https://%s%s", host, r.RequestURI)
				} else {
					target = fmt.Sprintf("https://%s:%d%s", host, httpsPort, r.RequestURI)
				}
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})
			srv := &http.Server{
				Addr:              addr,
				Handler:           s.tlsManager.HTTPHandler(redirectHandler),
				ReadTimeout:       10 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			errChan <- srv.ListenAndServe()
		}()
	}

	if httpsPort > 0 {
		started++
		go func() {
			addr := fmt.Sprintf(":%d", httpsPort)
			srv := &http.Server{
				Addr:              addr,
				Handler:           s.handler,
				TLSConfig:         s.tlsManager.GetTLSConfig(),
				ReadTimeout:       10 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			errChan <- srv.ListenAndServeTLS("", "")
		}()
	}

	if started == 0 {
		return fmt.Errorf("no ports configured to listen on")
	}

	firstErr := <-errChan
	go func() {
		if err := <-errChan; err != nil {
			log.Printf("server error: %v", err)
		}
	}()
	return firstErr
}
