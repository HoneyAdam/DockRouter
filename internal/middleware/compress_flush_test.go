package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompressFlush(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	compressHandler := Compress(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	compressHandler.ServeHTTP(rec, req)

	// Verify gzip response
	if encoding := rec.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Errorf("Content-Encoding = %s, want gzip", encoding)
	}

	// Decompress and verify content
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "hello" {
		t.Errorf("Body = %s, want hello", string(body))
	}
}

func TestCompressFlushWithoutGzip(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	compressHandler := Compress(handler)

	// No Accept-Encoding header
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	compressHandler.ServeHTTP(rec, req)

	if rec.Body.String() != "plain" {
		t.Errorf("Body = %s, want plain", rec.Body.String())
	}
}

func TestCompressWriteWithoutHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write without calling WriteHeader explicitly
		w.Write([]byte("auto-header"))
	})

	compressHandler := Compress(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	compressHandler.ServeHTTP(rec, req)

	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "auto-header" {
		t.Errorf("Body = %s, want auto-header", string(body))
	}
}
