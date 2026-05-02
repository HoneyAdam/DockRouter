package middleware

import (
	"testing"
)

func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewRateLimiter(1000, 60, 1000)
	defer rl.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.allow("10.0.0.1")
	}
}
