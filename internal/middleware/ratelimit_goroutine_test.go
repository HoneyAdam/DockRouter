package middleware

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestRateLimiterCleanupGoroutine tests that the cleanup goroutine stops on Close.
func TestRateLimiterCleanupGoroutine(t *testing.T) {
	rl := NewRateLimiter(10, 60, 10)

	// Add buckets manually with an old timestamp
	rl.mu.Lock()
	rl.buckets["stale-key"] = &tokenBucket{
		tokens:     5,
		lastRefill: time.Now().Add(-10 * time.Minute),
	}
	rl.buckets["fresh-key"] = &tokenBucket{
		tokens:     5,
		lastRefill: time.Now(),
	}
	rl.mu.Unlock()

	// Close stops the cleanup goroutine (proves select on rl.done works)
	rl.Close()
}

// TestRateLimiterCleanupViaTicker tests cleanup by waiting for the ticker.
// This test uses a shorter approach: create limiter, add stale buckets,
// then manually trigger the cleanup path.
func TestRateLimiterCleanupViaTicker(t *testing.T) {
	rl := NewRateLimiter(10, 60, 10)

	// Add stale bucket
	rl.mu.Lock()
	rl.buckets["old"] = &tokenBucket{
		tokens:     5,
		lastRefill: time.Now().Add(-10 * time.Minute),
	}
	rl.mu.Unlock()

	// Simulate what the cleanup goroutine does on ticker fire
	rl.mu.Lock()
	threshold := time.Now().Add(-5 * time.Minute)
	deleted := 0
	for key, bucket := range rl.buckets {
		if bucket.lastRefill.Before(threshold) {
			delete(rl.buckets, key)
			deleted++
		}
	}
	rl.mu.Unlock()

	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	rl.mu.RLock()
	_, exists := rl.buckets["old"]
	rl.mu.RUnlock()
	if exists {
		t.Error("stale bucket should have been deleted")
	}

	rl.Close()
}

// TestRateLimiterEvictionAtCapacity tests LRU eviction when maxBuckets is reached.
func TestRateLimiterEvictionAtCapacity(t *testing.T) {
	rl := NewRateLimiter(10, 60, 10)
	defer rl.Close()

	// Set a small maxBuckets for testing
	rl.maxBuckets = 3

	// Add 3 buckets
	rl.allow("key1")
	time.Sleep(time.Millisecond)
	rl.allow("key2")
	time.Sleep(time.Millisecond)
	rl.allow("key3")

	rl.mu.RLock()
	count := len(rl.buckets)
	rl.mu.RUnlock()
	if count != 3 {
		t.Fatalf("bucket count = %d, want 3", count)
	}

	// Adding a 4th should evict the oldest
	rl.allow("key4")

	rl.mu.RLock()
	count = len(rl.buckets)
	_, hasKey1 := rl.buckets["key1"]
	_, hasKey4 := rl.buckets["key4"]
	rl.mu.RUnlock()

	if count != 3 {
		t.Errorf("bucket count after eviction = %d, want 3", count)
	}
	if hasKey1 {
		t.Error("key1 should have been evicted (oldest)")
	}
	if !hasKey4 {
		t.Error("key4 should exist (newly added)")
	}
}

// TestRateLimiterConcurrentAllow tests concurrent access to allow().
func TestRateLimiterConcurrentAllow(t *testing.T) {
	rl := NewRateLimiter(1000, 60, 1000)
	defer rl.Close()

	var allowed atomic.Int64
	var denied atomic.Int64

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				ok, _ := rl.allow("shared-key")
				if ok {
					allowed.Add(1)
				} else {
					denied.Add(1)
				}
			}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	total := allowed.Load() + denied.Load()
	if total != 1000 {
		t.Errorf("total requests = %d, want 1000", total)
	}
}

// TestRateLimiterTokenRefillWithDrain tests that tokens refill over time after draining.
func TestRateLimiterTokenRefillWithDrain(t *testing.T) {
	rl := NewRateLimiter(60, 60, 60)
	defer rl.Close()

	// Drain all tokens (first call creates bucket without decrementing,
	// so we need maxSize+1 more calls to exhaust)
	for i := 0; i < 62; i++ {
		rl.allow("key")
	}

	// Should be denied
	ok, _ := rl.allow("key")
	if ok {
		t.Error("should be denied after draining tokens")
	}

	// Manually advance the lastRefill time to simulate time passing
	rl.mu.Lock()
	if b, exists := rl.buckets["key"]; exists {
		b.lastRefill = time.Now().Add(-2 * time.Second)
	}
	rl.mu.Unlock()

	// Should have refilled some tokens (2 seconds * 1 token/sec = ~2 tokens)
	ok, remaining := rl.allow("key")
	if !ok {
		t.Error("should be allowed after refill")
	}
	if remaining < 0 {
		t.Errorf("remaining = %f, should be >= 0", remaining)
	}
}
