package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewRateLimiterZeroWindow tests NewRateLimiter with zero window defaults to 60s.
func TestNewRateLimiterZeroWindow(t *testing.T) {
	rl := NewRateLimiter(100, 0, 0)
	defer rl.Close()

	if rl.window != 60 {
		t.Errorf("window = %d, want 60 (default)", rl.window)
	}
	if rl.maxSize != 100 {
		t.Errorf("maxSize = %d, want 100 (default=rate)", rl.maxSize)
	}
}

// TestNewRateLimiterZeroMaxSize tests NewRateLimiter with zero maxSize defaults to rate.
func TestNewRateLimiterZeroMaxSize(t *testing.T) {
	rl := NewRateLimiter(50, 30, 0)
	defer rl.Close()

	if rl.maxSize != 50 {
		t.Errorf("maxSize = %d, want 50 (default=rate)", rl.maxSize)
	}
}

// TestNewRateLimiterCustomMaxSize tests NewRateLimiter with custom maxSize.
func TestNewRateLimiterCustomMaxSize(t *testing.T) {
	rl := NewRateLimiter(100, 60, 200)
	defer rl.Close()

	if rl.maxSize != 200 {
		t.Errorf("maxSize = %d, want 200 (custom)", rl.maxSize)
	}
}

// TestNewRateLimiterRefillRate tests refill rate calculation.
func TestNewRateLimiterRefillRate(t *testing.T) {
	rl := NewRateLimiter(60, 60, 60)
	defer rl.Close()

	expectedRefill := float64(60) / float64(60)
	if rl.refillRate != expectedRefill {
		t.Errorf("refillRate = %f, want %f", rl.refillRate, expectedRefill)
	}
}

// TestRateLimiterBurstWithHigherMaxSize tests that burst allows more requests than rate.
func TestRateLimiterBurstWithHigherMaxSize(t *testing.T) {
	rl := NewRateLimiter(10, int(time.Minute.Seconds()), 20)
	defer rl.Close()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowed := 0
	for i := 0; i < 25; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			allowed++
		}
	}

	// Should allow more than rate=10 due to maxSize=20 burst
	if allowed <= 10 {
		t.Errorf("allowed = %d, want > 10 (burst should exceed rate)", allowed)
	}
}
