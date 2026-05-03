package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDoStreamRequestSuccess tests doStreamRequest with a streaming mock server.
func TestDoStreamRequestSuccess(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.53/events" {
			w.Header().Set("Content-Type", "application/json")
			// Write two events
			event1 := `{"Type":"container","Action":"start","Actor":{"ID":"abc123"},"time":1700000000}`
			event2 := `{"Type":"container","Action":"stop","Actor":{"ID":"def456"},"time":1700000001}`
			w.Write([]byte(event1 + "\n"))
			w.Write([]byte(event2 + "\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	reader, err := client.doStreamRequest(ctx, http.MethodGet, "/events")
	if err != nil {
		t.Fatalf("doStreamRequest error: %v", err)
	}
	defer reader.Close()

	// Read events from stream
	scanner := bufio.NewScanner(reader)
	eventCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		eventCount++
		if eventCount >= 2 {
			break
		}
	}

	if eventCount != 2 {
		t.Errorf("event count = %d, want 2", eventCount)
	}
}

// TestDoStreamRequestErrorStatus tests doStreamRequest with non-2xx response.
func TestDoStreamRequestErrorStatus(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	_, err = client.doStreamRequest(ctx, http.MethodGet, "/events")
	if err == nil {
		t.Fatal("doStreamRequest should fail with 500 response")
	}
	if !strings.Contains(err.Error(), "docker API error") {
		t.Errorf("error = %v, want docker API error", err)
	}
}

// TestDoStreamRequestConnectionError tests doStreamRequest with non-existent socket.
func TestDoStreamRequestConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/tmp/nonexistent_docker_socket_test.sock")
	defer client.Close()

	ctx := context.Background()
	_, err := client.doStreamRequest(ctx, http.MethodGet, "/events")
	if err == nil {
		t.Fatal("doStreamRequest should fail with non-existent socket")
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("error = %v, want connection error", err)
	}
}

// TestUnixReadCloserCloseViaStream tests the unixReadCloser Close method via doStreamRequest.
func TestUnixReadCloserCloseViaStream(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Type":"container","Action":"start"}`))
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	reader, err := client.doStreamRequest(ctx, http.MethodGet, "/events")
	if err != nil {
		t.Fatalf("doStreamRequest error: %v", err)
	}

	// Close should not panic
	if err := reader.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
}

// TestWatchEventsReconnectPath tests watchEvents reconnect when events channel closes.
func TestWatchEventsReconnectPath(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.53/containers/json" {
			json.NewEncoder(w).Encode([]Container{})
			return
		}
		if r.URL.Path == "/v1.53/events" {
			w.Header().Set("Content-Type", "application/json")
			// Send one event then close (triggers reconnect)
			w.Write([]byte(`{"Type":"container","Action":"start","Actor":{"ID":"abc123"},"time":1700000000}` + "\n"))
			return
		}
		json.NewEncoder(w).Encode([]Event{})
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	logger := &mockDiscoveryLogger{}
	sink := newMockRouteSink()

	engine := NewEngine(client, sink, logger)

	// Use a short context to limit reconnects
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		engine.watchEvents(ctx)
		close(done)
	}()

	// Wait for context to expire
	select {
	case <-done:
		// Good — watchEvents returned
	case <-time.After(5 * time.Second):
		t.Fatal("watchEvents should return when context expires")
	}
}

// mockDiscoveryLogger is a logger for discovery tests
type mockDiscoveryLogger struct{}

func (m *mockDiscoveryLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockDiscoveryLogger) Info(msg string, fields ...interface{})  {}
func (m *mockDiscoveryLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockDiscoveryLogger) Error(msg string, fields ...interface{}) {}
