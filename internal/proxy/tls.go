package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/niix-dan/apexproxy/internal/config"
	"golang.org/x/crypto/acme/autocert"
)

const maxSelfSignedCacheSize = 128

type TLSManager struct {
	globalCert      *tls.Certificate
	routeCerts      map[string]*tls.Certificate
	autoTLS         *autocert.Manager
	selfSignedMu    sync.Mutex
	selfSignedCache map[string]*tls.Certificate
	knownHosts      map[string]struct{}
}

func NewTLSManager(cfg *config.Config) (*TLSManager, error) {
	manager := &TLSManager{
		routeCerts:      make(map[string]*tls.Certificate),
		selfSignedCache: make(map[string]*tls.Certificate),
		knownHosts:      make(map[string]struct{}),
	}

	for _, route := range cfg.Routing {
		if route.Host != "" {
			manager.knownHosts[route.Host] = struct{}{}
		}
	}

	if cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load global TLS cert: %w", err)
		}
		manager.globalCert = &cert
	}

	for _, route := range cfg.Routing {
		if route.TLS.CertFile != "" && route.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(route.TLS.CertFile, route.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load TLS cert for route %q: %w", route.Host, err)
			}
			manager.routeCerts[route.Host] = &cert
		}
	}

	if cfg.Server.AutoTLS {
		manager.autoTLS = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache("certs"),
			HostPolicy: autocert.HostWhitelist(collectHosts(cfg)...),
		}
	}

	return manager, nil
}

func (m *TLSManager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.getCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

func (m *TLSManager) HTTPHandler(fallback http.Handler) http.Handler {
	if m.autoTLS != nil {
		return m.autoTLS.HTTPHandler(fallback)
	}
	return fallback
}

func (m *TLSManager) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := hello.ServerName

	if cert, ok := m.routeCerts[name]; ok {
		return cert, nil
	}

	for host, cert := range m.routeCerts {
		if strings.HasPrefix(host, "*.") && strings.HasSuffix(name, host[1:]) {
			return cert, nil
		}
	}

	if m.autoTLS != nil {
		if cert, err := m.autoTLS.GetCertificate(hello); err == nil {
			return cert, nil
		}
	}

	if m.globalCert != nil {
		return m.globalCert, nil
	}

	if !m.isKnownHost(name) {
		return nil, fmt.Errorf("TLS: rejecting unknown server name %q", name)
	}

	return m.getOrCreateSelfSigned(hello.ServerName)
}

func (m *TLSManager) isKnownHost(name string) bool {
	if _, ok := m.knownHosts[name]; ok {
		return true
	}
	for h := range m.knownHosts {
		if strings.HasPrefix(h, "*.") && strings.HasSuffix(name, h[1:]) {
			return true
		}
	}
	return false
}

func (m *TLSManager) getOrCreateSelfSigned(host string) (*tls.Certificate, error) {
	m.selfSignedMu.Lock()
	defer m.selfSignedMu.Unlock()

	if cert, ok := m.selfSignedCache[host]; ok {
		return cert, nil
	}

	if len(m.selfSignedCache) >= maxSelfSignedCacheSize {
		return nil, fmt.Errorf("TLS: self-signed cert cache full, refusing %q", host)
	}

	cert, err := generateSelfSigned(host)
	if err != nil {
		return nil, fmt.Errorf("failed to generate self-signed cert for %q: %w", host, err)
	}

	m.selfSignedCache[host] = cert
	return cert, nil
}

func generateSelfSigned(host string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
	return tlsCert, nil
}

func collectHosts(cfg *config.Config) []string {
	seen := make(map[string]struct{})
	var hosts []string
	for _, route := range cfg.Routing {
		if route.Host != "" && !strings.HasPrefix(route.Host, "*.") {
			if _, ok := seen[route.Host]; !ok {
				seen[route.Host] = struct{}{}
				hosts = append(hosts, route.Host)
			}
		}
	}
	return hosts
}
