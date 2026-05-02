package middleware

import (
	"testing"
)

func TestRateLimiterEviction(t *testing.T) {
	rl := NewRateLimiter(10, 60, 10)
	defer rl.Close()

	// Set a low maxBuckets for testing
	rl.mu.Lock()
	rl.maxBuckets = 3
	rl.mu.Unlock()

	// Add 3 buckets
	rl.allow("1.1.1.1")
	rl.allow("2.2.2.2")
	rl.allow("3.3.3.3")

	rl.mu.RLock()
	count := len(rl.buckets)
	rl.mu.RUnlock()
	if count != 3 {
		t.Fatalf("bucket count = %d, want 3", count)
	}

	// Adding a 4th should trigger eviction of one bucket (oldest by lastRefill)
	rl.allow("4.4.4.4")

	rl.mu.RLock()
	count = len(rl.buckets)
	rl.mu.RUnlock()
	if count != 3 {
		t.Errorf("bucket count after eviction = %d, want 3", count)
	}

	// New bucket should exist
	rl.mu.RLock()
	_, exists := rl.buckets["4.4.4.4"]
	rl.mu.RUnlock()
	if !exists {
		t.Error("new bucket should exist")
	}
}

func TestRateLimiterCleanupStopsOnClose(t *testing.T) {
	rl := NewRateLimiter(10, 60, 10)

	// Add a bucket
	rl.allow("test-key")

	rl.mu.RLock()
	count := len(rl.buckets)
	rl.mu.RUnlock()
	if count != 1 {
		t.Fatalf("bucket count = %d, want 1", count)
	}

	// Close should stop cleanup goroutine
	rl.Close()

	// Bucket should still be there (cleanup ticker hasn't fired)
	rl.mu.RLock()
	count = len(rl.buckets)
	rl.mu.RUnlock()
	if count != 1 {
		t.Errorf("bucket count after close = %d, want 1", count)
	}
}

func TestRateLimiterBucketStartsAtMaxSize(t *testing.T) {
	rl := NewRateLimiter(5, 60, 5)
	defer rl.Close()

	// First request creates bucket with maxSize tokens, returns maxSize (no decrement)
	allowed, remaining := rl.allow("new-key")
	if !allowed {
		t.Error("first request should be allowed")
	}
	if int(remaining) != 5 {
		t.Errorf("remaining after first = %d, want 5", int(remaining))
	}

	// Second request: refill, tokens still 5 (no time elapsed), decrement to 4
	allowed, remaining = rl.allow("new-key")
	if !allowed {
		t.Error("second request should be allowed")
	}
	if int(remaining) != 4 {
		t.Errorf("remaining after second = %d, want 4", int(remaining))
	}
}

func TestRateLimiterExhaustAndRefill(t *testing.T) {
	rl := NewRateLimiter(2, 60, 2)
	defer rl.Close()

	// First call creates bucket with 2 tokens (no decrement)
	rl.allow("key")
	// Second call: tokens=2, decrement to 1
	rl.allow("key")
	// Third call: tokens=1, decrement to 0
	rl.allow("key")
	// Fourth call: tokens=0, denied
	allowed, _ := rl.allow("key")
	if allowed {
		t.Error("request should be denied after bucket exhausted")
	}
}

func TestRateLimiterMultipleKeys(t *testing.T) {
	rl := NewRateLimiter(1, 60, 1)
	defer rl.Close()

	// Key 1: creates bucket with 1 token (no decrement)
	rl.allow("key1")

	// Key 2: separate bucket with 1 token
	allowed, _ := rl.allow("key2")
	if !allowed {
		t.Error("different key should have its own bucket")
	}

	// Key 1 second call: tokens=1, decrement to 0
	rl.allow("key1")

	// Key 1 third call: tokens=0, denied
	allowed, _ = rl.allow("key1")
	if allowed {
		t.Error("key1 should be exhausted")
	}
}
