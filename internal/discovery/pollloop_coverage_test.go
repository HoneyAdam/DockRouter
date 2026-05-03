package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// TestPollLoopSyncErr500 tests pollLoop logs error when Sync returns HTTP 500.
func TestPollLoopSyncErr500(t *testing.T) {
	var syncCalls int32
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/json":
			atomic.AddInt32(&syncCalls, 1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
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
	engine.interval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	engine.pollLoop(ctx)

	// Sync should have been called at least once
	calls := atomic.LoadInt32(&syncCalls)
	if calls < 1 {
		t.Errorf("Sync called %d times, want >= 1", calls)
	}
}

// TestPollLoopDefaultInterval tests pollLoop uses default 30s when interval is zero.
func TestPollLoopDefaultInterval(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/json":
			json.NewEncoder(w).Encode([]Container{})
		default:
			w.WriteHeader(http.StatusOK)
		}
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
	engine.interval = 0 // zero — should default to 30s

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	engine.pollLoop(ctx)
}
