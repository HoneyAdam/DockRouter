package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGzipResponseWriterWriteHeaderNormal tests the normal WriteHeader path
// that initializes gzip compression.
func TestGzipResponseWriterWriteHeaderNormal(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/html; charset=utf-8")

	w := &gzipResponseWriter{ResponseWriter: rec}

	w.WriteHeader(http.StatusOK)

	if !w.wroteHeader {
		t.Error("wroteHeader should be true after WriteHeader")
	}
	if w.writer == nil {
		t.Error("writer should be initialized for compressible content type")
	}
	if ct := rec.Header().Get("Content-Encoding"); ct != "gzip" {
		t.Errorf("Content-Encoding = %s, want gzip", ct)
	}
}

// TestGzipResponseWriterWriteHeaderDuplicate tests that duplicate calls are no-ops.
func TestGzipResponseWriterWriteHeaderDuplicate(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/plain")

	w := &gzipResponseWriter{ResponseWriter: rec}
	w.WriteHeader(http.StatusOK)

	// Reset to track if WriteHeader is called again on inner
	w.WriteHeader(http.StatusNotFound)

	if rec.Code != http.StatusOK {
		t.Errorf("Code = %d, want 200 (first WriteHeader wins)", rec.Code)
	}
}

// TestGzipResponseWriterWriteHeaderNoContent tests 204 skips compression.
func TestGzipResponseWriterWriteHeaderNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/plain")

	w := &gzipResponseWriter{ResponseWriter: rec}
	w.WriteHeader(http.StatusNoContent)

	if w.writer != nil {
		t.Error("writer should NOT be initialized for 204")
	}
	if enc := rec.Header().Get("Content-Encoding"); enc == "gzip" {
		t.Error("should not set gzip for 204")
	}
}

// TestGzipResponseWriterWriteHeaderNotModified tests 304 skips compression.
func TestGzipResponseWriterWriteHeaderNotModified(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/plain")

	w := &gzipResponseWriter{ResponseWriter: rec}
	w.WriteHeader(http.StatusNotModified)

	if w.writer != nil {
		t.Error("writer should NOT be initialized for 304")
	}
}

// TestGzipResponseWriterWriteHeaderNonCompressible tests non-compressible types skip gzip.
func TestGzipResponseWriterWriteHeaderNonCompressible(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "image/png")

	w := &gzipResponseWriter{ResponseWriter: rec}
	w.WriteHeader(http.StatusOK)

	if w.writer != nil {
		t.Error("writer should NOT be initialized for image/png")
	}
}

// TestGzipResponseWriterWriteAutoInit tests Write auto-calls WriteHeader(200).
func TestGzipResponseWriterWriteAutoInit(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")

	w := &gzipResponseWriter{ResponseWriter: rec}

	data := []byte(`{"status":"ok"}`)
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
	if !w.wroteHeader {
		t.Error("Write should have auto-called WriteHeader")
	}
}

// TestGzipResponseWriterWriteWithoutGzip writes without gzip init (non-compressible).
func TestGzipResponseWriterWriteWithoutGzip(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "image/jpeg")

	w := &gzipResponseWriter{ResponseWriter: rec}

	data := []byte("fake-jpeg-data")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
	// Data should go directly to underlying ResponseWriter
	if !strings.Contains(rec.Body.String(), "fake-jpeg-data") {
		t.Errorf("body = %s, want raw data", rec.Body.String())
	}
}

// TestCompressEndToEnd tests the full Compress middleware pipeline.
func TestCompressEndToEnd(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello world"))
	})

	handler := Compress(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("Content-Encoding = %s, want gzip", enc)
	}

	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}
	if string(decompressed) != "hello world" {
		t.Errorf("body = %q, want 'hello world'", string(decompressed))
	}
}

// TestCompressNoGzipAcceptEncoding tests passthrough when client doesn't accept gzip.
func TestCompressNoGzipAcceptEncoding(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain response"))
	})

	handler := Compress(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if enc := rec.Header().Get("Content-Encoding"); enc == "gzip" {
		t.Error("should not gzip when client doesn't accept it")
	}
	if rec.Body.String() != "plain response" {
		t.Errorf("body = %q, want plain", rec.Body.String())
	}
}

// TestIsCompressible tests the compressible content type checker.
func TestIsCompressible(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/html", true},
		{"text/plain; charset=utf-8", true},
		{"application/json", true},
		{"application/javascript", true},
		{"application/xml", true},
		{"image/png", false},
		{"image/jpeg", false},
		{"video/mp4", false},
		{"application/octet-stream", false},
		{"", true}, // default to compress
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isCompressible(tt.ct)
			if got != tt.want {
				t.Errorf("isCompressible(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

// TestGzipResponseWriterFlush tests the Flush method.
func TestGzipResponseWriterFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &gzipResponseWriter{ResponseWriter: rec}

	// Flush without gzip writer (should not panic)
	w.Flush()

	// Initialize gzip writer and flush
	w.init()
	w.Flush()
}
