package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStripPrefixMismatch tests StripPrefix when path doesn't match prefix.
func TestStripPrefixMismatch(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := StripPrefix("/api")(inner)

	req := httptest.NewRequest("GET", "/web/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for non-matching prefix", rec.Code)
	}
}

// TestStripPrefixEmptyResult tests StripPrefix when result path becomes empty.
func TestStripPrefixEmptyResult(t *testing.T) {
	var receivedPath string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	handler := StripPrefix("/api")(inner)

	req := httptest.NewRequest("GET", "/api", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if receivedPath != "/" {
		t.Errorf("path = %q, want /", receivedPath)
	}
}

// TestStripPrefixWithRawPath tests StripPrefix with URL-encoded raw path.
func TestStripPrefixWithRawPath(t *testing.T) {
	var receivedPath string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	handler := StripPrefix("/api")(inner)

	req := httptest.NewRequest("GET", "/api/v2/users", nil)
	req.URL.RawPath = "/api/v2/users"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if receivedPath != "/v2/users" {
		t.Errorf("path = %q, want /v2/users", receivedPath)
	}
}

// TestAddPrefixBasic tests AddPrefix middleware.
func TestAddPrefixBasic(t *testing.T) {
	var receivedPath string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	handler := AddPrefix("/v2")(inner)

	req := httptest.NewRequest("GET", "/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if receivedPath != "/v2/users" {
		t.Errorf("path = %q, want /v2/users", receivedPath)
	}
}

// TestAddPrefixWithRawPath tests AddPrefix with URL-encoded raw path.
func TestAddPrefixWithRawPath(t *testing.T) {
	var receivedPath string
	var receivedRawPath string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedRawPath = r.URL.RawPath
		w.WriteHeader(http.StatusOK)
	})
	handler := AddPrefix("/api")(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.URL.RawPath = "/test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedPath != "/api/test" {
		t.Errorf("path = %q, want /api/test", receivedPath)
	}
	if receivedRawPath != "/api/test" {
		t.Errorf("rawPath = %q, want /api/test", receivedRawPath)
	}
}

// TestRedirectHTTPSAlreadyTLS tests RedirectHTTPS when request is already TLS.
func TestRedirectHTTPSAlreadyTLS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RedirectHTTPS(nil)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{} // Simulate TLS
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (already TLS)", rec.Code)
	}
}

// TestRedirectHTTPSForwardedProto tests RedirectHTTPS with X-Forwarded-Proto.
func TestRedirectHTTPSForwardedProto(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RedirectHTTPS(nil)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (forwarded proto https)", rec.Code)
	}
}

// TestRedirectHTTPSInvalidHost tests RedirectHTTPS with disallowed host.
func TestRedirectHTTPSInvalidHost(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	allowed := map[string]bool{"example.com": true}
	handler := RedirectHTTPS(allowed)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid host)", rec.Code)
	}
}

// TestRedirectHTTPSAllowedHost tests RedirectHTTPS with allowed host.
func TestRedirectHTTPSAllowedHost(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	allowed := map[string]bool{"example.com": true}
	handler := RedirectHTTPS(allowed)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
}

// TestRedirectHTTPSPreservesQuery tests redirect preserves query string.
func TestRedirectHTTPSPreservesQuery(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := RedirectHTTPS(nil)(inner)

	req := httptest.NewRequest("GET", "/test?foo=bar", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	loc := rec.Header().Get("Location")
	if loc != "https://example.com/test?foo=bar" {
		t.Errorf("Location = %s, want https://example.com/test?foo=bar", loc)
	}
}

// TestRedirectHTTPSWithPort tests redirect strips port from host.
func TestRedirectHTTPSWithPort(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	allowed := map[string]bool{"example.com": true}
	handler := RedirectHTTPS(allowed)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
}
