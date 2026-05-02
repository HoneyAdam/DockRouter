package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOnlyRejectsNonGet(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("inner handler should not be called")
			})

			handler := getOnly(inner)
			req := httptest.NewRequest(method, "/api/v1/status", nil)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if rec.Body.String() != "Method not allowed\n" {
				t.Errorf("Body = %q, want 'Method not allowed\\n'", rec.Body.String())
			}
		})
	}
}

func TestGetOnlyAllowsGet(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := getOnly(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !called {
		t.Error("inner handler should be called for GET")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetOnlyHeadAllowed(t *testing.T) {
	// HEAD should be rejected too since it's not GET
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := getOnly(inner)
	req := httptest.NewRequest(http.MethodHead, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if called {
		t.Error("inner handler should not be called for HEAD")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
