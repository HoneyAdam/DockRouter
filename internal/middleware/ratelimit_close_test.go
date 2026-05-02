package middleware

import (
	"testing"
)

func TestRateLimiterClose(t *testing.T) {
	limiter := NewRateLimiter(10, 60, 10)

	// Use the limiter a bit
	limiter.allow("key1")
	limiter.allow("key2")

	// Close should stop the cleanup goroutine without panic or deadlock
	limiter.Close()

	// Verify done channel is closed
	select {
	case <-limiter.done:
		// Expected: channel is closed
	default:
		t.Error("Close() did not close the done channel")
	}
}

func TestRateLimiterCloseIdempotentPanics(t *testing.T) {
	limiter := NewRateLimiter(10, 60, 10)

	limiter.Close()

	// Second close should panic (closing already-closed channel)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on double close")
		}
	}()
	limiter.Close()
}
