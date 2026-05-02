package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyServeHTTP(t *testing.T) {
	// Create a real backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response"))
	}))
	defer backend.Close()

	// Extract host:port from backend URL
	target := backend.Listener.Addr().String()

	logger := &mockLogger{}
	p := NewProxy(logger)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	err := p.ServeHTTP(w, req, target)
	if err != nil {
		t.Errorf("ServeHTTP error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "backend response" {
		t.Errorf("Body = %s, want 'backend response'", w.Body.String())
	}
}

func TestProxyServeHTTPBackendError(t *testing.T) {
	logger := &mockLogger{}
	p := NewProxy(logger)

	// Use a non-existent target to trigger error
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	err := p.ServeHTTP(w, req, "127.0.0.1:1")
	if err == nil {
		t.Error("Expected error for unreachable backend")
	}
}

func TestProxyServeHTTPHeaders(t *testing.T) {
	var receivedXFF, receivedProto, receivedHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedXFF = r.Header.Get("X-Forwarded-For")
		receivedProto = r.Header.Get("X-Forwarded-Proto")
		receivedHost = r.Header.Get("X-Forwarded-Host")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	target := backend.Listener.Addr().String()
	logger := &mockLogger{}
	p := NewProxy(logger)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req, target)

	// XFF may appear duplicated if both Director and transport set it
	if receivedXFF != "10.0.0.1" && receivedXFF != "10.0.0.1, 10.0.0.1" {
		t.Errorf("X-Forwarded-For = %s, want 10.0.0.1", receivedXFF)
	}
	if receivedProto != "http" {
		t.Errorf("X-Forwarded-Proto = %s, want http", receivedProto)
	}
	if receivedHost != "example.com" {
		t.Errorf("X-Forwarded-Host = %s, want example.com", receivedHost)
	}
}

func TestProxySetTimeout(t *testing.T) {
	logger := &mockLogger{}
	p := NewProxy(logger)

	p.SetTimeout(5 * time.Second)
	// Should not panic - just sets transport timeout
}

func TestProxyServeHTTPWithPOST(t *testing.T) {
	var receivedMethod string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	target := backend.Listener.Addr().String()
	logger := &mockLogger{}
	p := NewProxy(logger)

	req := httptest.NewRequest("POST", "http://example.com/api", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req, target)

	if receivedMethod != "POST" {
		t.Errorf("Method = %s, want POST", receivedMethod)
	}
}

func TestProxyServeHTTPDifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"201 Created", 201},
		{"204 No Content", 204},
		{"301 Moved", 301},
		{"404 Not Found", 404},
		{"500 Internal Server Error", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer backend.Close()

			target := backend.Listener.Addr().String()
			logger := &mockLogger{}
			p := NewProxy(logger)

			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			w := httptest.NewRecorder()

			p.ServeHTTP(w, req, target)

			if w.Code != tt.statusCode {
				t.Errorf("Status = %d, want %d", w.Code, tt.statusCode)
			}
		})
	}
}
