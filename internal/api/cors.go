package api

import (
	"net/http"
	"strings"
)

// CORSPolicy lets a web console served by one Ring Promoter talk to another,
// so an operator can switch between clusters in one browser session.
//
// The API authenticates with a bearer token, never a cookie, so answering a
// foreign origin grants nothing by itself: the page must still hold that
// server's token. Credentials are therefore never allowed (no
// Access-Control-Allow-Credentials), and the default is to accept any origin.
// RP_CORS_ORIGINS narrows it to a comma-separated list of origins, or "off"
// turns cross-origin access off entirely.
type CORSPolicy struct {
	disabled bool
	any      bool
	origins  map[string]bool
}

// ParseCORSOrigins reads an RP_CORS_ORIGINS value. Empty or "*" accepts any
// origin; "off" or "none" disables CORS; anything else is a comma-separated
// list of exact origins such as https://rp.example.com.
func ParseCORSOrigins(v string) CORSPolicy {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "*":
		return CORSPolicy{any: true}
	case "off", "none", "false":
		return CORSPolicy{disabled: true}
	}
	p := CORSPolicy{origins: map[string]bool{}}
	for _, o := range strings.Split(v, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "*" {
			return CORSPolicy{any: true}
		}
		if o != "" {
			p.origins[o] = true
		}
	}
	return p
}

// SetCORSOrigins replaces the default (any origin) cross-origin policy.
func (s *Server) SetCORSOrigins(p CORSPolicy) { s.cors = p }

func (p CORSPolicy) allows(origin string) bool {
	if p.disabled || origin == "" {
		return false
	}
	return p.any || p.origins[origin]
}

// corsPath reports the routes a console on another origin needs. The UI and
// /metrics are left alone.
func corsPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/version" || path == "/healthz"
}

// withCORS answers preflights before authentication (a preflight never
// carries the bearer token) and marks real responses readable by an allowed
// origin.
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !corsPath(r.URL.Path) || !s.cors.allows(origin) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		h.Set("Access-Control-Allow-Origin", origin)
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Actor")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
