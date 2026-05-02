// Package middleware provides HTTP middleware components
package middleware

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"time"
)

// RequestIDHeader is the header name for request IDs
const RequestIDHeader = "X-Request-Id"

// RequestID generates a unique request ID and adds it to headers
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use existing ID if present
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = generateID()
			r.Header.Set(RequestIDHeader, id)
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp + nanoseconds for uniqueness
		now := time.Now()
		binary.BigEndian.PutUint64(b[0:8], uint64(now.Unix()))
		binary.BigEndian.PutUint64(b[8:16], uint64(now.Nanosecond()))
	}
	return hex.EncodeToString(b)
}
