package common

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

type IPAllowlist struct {
	mu      sync.RWMutex
	cidrs   []*net.IPNet
	enabled bool
}

func NewIPAllowlist() *IPAllowlist {
	return &IPAllowlist{}
}

func (a *IPAllowlist) AddCIDR(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.cidrs = append(a.cidrs, network)
	a.mu.Unlock()
	return nil
}

func (a *IPAllowlist) RemoveCIDR(cidr string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, c := range a.cidrs {
		if c.String() == cidr {
			a.cidrs = append(a.cidrs[:i], a.cidrs[i+1:]...)
			return
		}
	}
}

func (a *IPAllowlist) SetEnabled(enabled bool) { a.mu.Lock(); a.enabled = enabled; a.mu.Unlock() }

func (a *IPAllowlist) IsAllowed(ip string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.enabled || len(a.cidrs) == 0 {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range a.cidrs {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

func (a *IPAllowlist) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		if !a.IsAllowed(ip) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
