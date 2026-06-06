package middlewares

import (
	"fmt"
	"net"
	"net/http"
	"sync"
)

type IPFilter struct {
	mu        sync.RWMutex
	blacklist []*net.IPNet
	whitelist []*net.IPNet
}

func NewIPFilter(blacklistCIDRs, whitelistCIDRs []string) (*IPFilter, error) {
	f := &IPFilter{}
	for _, cidr := range blacklistCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid blacklist CIDR %q: %w", cidr, err)
		}
		f.blacklist = append(f.blacklist, n)
	}
	for _, cidr := range whitelistCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid whitelist CIDR %q: %w", cidr, err)
		}
		f.whitelist = append(f.whitelist, n)
	}
	return f, nil
}

func (f *IPFilter) Allow(ip net.IP) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.whitelist) > 0 {
		found := false
		for _, n := range f.whitelist {
			if n.Contains(ip) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, n := range f.blacklist {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func (f *IPFilter) Block(cidr string) error {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	f.mu.Lock()
	f.blacklist = append(f.blacklist, n)
	f.mu.Unlock()
	return nil
}

func (f *IPFilter) Unblock(cidr string) error {
	_, target, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.blacklist[:0]
	for _, n := range f.blacklist {
		if n.String() != target.String() {
			out = append(out, n)
		}
	}
	f.blacklist = out
	return nil
}

func (f *IPFilter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ipStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ipStr = r.RemoteAddr
		}
		ip := net.ParseIP(ipStr)
		if ip == nil || !f.Allow(ip) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
