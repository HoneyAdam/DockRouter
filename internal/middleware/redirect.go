// Package middleware provides HTTP middleware components
package middleware

import (
	"net"
	"net/http"
	"strings"
)

// RedirectHTTPS redirects HTTP requests to HTTPS.
// The allowedHosts set validates the Host header to prevent open redirects.
// If allowedHosts is nil, all hosts are allowed (not recommended for production).
func RedirectHTTPS(allowedHosts map[string]bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if already HTTPS
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				next.ServeHTTP(w, r)
				return
			}

			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.ToLower(host)

			if len(allowedHosts) > 0 && !allowedHosts[host] {
				http.Error(w, "Invalid host", http.StatusBadRequest)
				return
			}

			target := "https://" + r.Host + r.URL.Path
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})
	}
}
