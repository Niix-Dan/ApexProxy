package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
)

type Server struct {
	cfg        *config.Config
	router     *Router
	tlsManager *TLSManager
}

func NewServer(cfg *config.Config) (*Server, error) {
	tlsManager, err := NewTLSManager(cfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:        cfg,
		router:     NewRouter(cfg),
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
				Addr:         addr,
				Handler:      s.tlsManager.HTTPHandler(redirectHandler),
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  120 * time.Second,
			}
			errChan <- srv.ListenAndServe()
		}()
	}

	if httpsPort > 0 {
		started++
		go func() {
			addr := fmt.Sprintf(":%d", httpsPort)
			srv := &http.Server{
				Addr:         addr,
				Handler:      s.router,
				TLSConfig:    s.tlsManager.GetTLSConfig(),
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  120 * time.Second,
			}
			errChan <- srv.ListenAndServeTLS("", "")
		}()
	}

	if started == 0 {
		return fmt.Errorf("no ports configured")
	}

	firstErr := <-errChan
	go func() {
		if err := <-errChan; err != nil {
			log.Printf("server error: %v", err)
		}
	}()
	return firstErr
}
