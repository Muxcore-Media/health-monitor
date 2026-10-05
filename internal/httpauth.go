package internal

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// isLoopbackAddr reports whether a listen address binds only to loopback.
// Empty host (":9203"), wildcard hosts and non-"localhost" names are non-loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validateHTTPListen enforces NFR-SEC-011: non-loopback HTTP requires a bearer token.
func validateHTTPListen(addr, token string) error {
	if !isLoopbackAddr(addr) && token == "" {
		return fmt.Errorf("refusing to serve HTTP on non-loopback address %q without HEALTH_MONITOR_HTTP_TOKEN (set a bearer token or bind to 127.0.0.1)", addr)
	}
	return nil
}

func tokenMatches(want, got string) bool {
	a := sha256.Sum256([]byte(want))
	b := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func bearerToken(h string) string {
	const p = "bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// requireToken wraps next so every path except GET/HEAD /health requires a
// bearer token. An empty token disables the check (loopback-only deployments).
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearerToken(r.Header.Get("Authorization"))
		if got == "" || !tokenMatches(token, got) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="health-monitor"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// httpHandler returns the module HTTP handler with auth applied.
func (m *Module) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/status", m.handleStatus)
	return requireToken(m.httpToken, mux)
}
