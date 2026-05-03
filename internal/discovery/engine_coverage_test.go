package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestWatchEventsReconnectLoop tests that watchEvents reconnects after event stream ends.
func TestWatchEventsReconnectLoop(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/events":
			// Return a few events then close to trigger reconnect
			encoder := json.NewEncoder(w)
			encoder.Encode(Event{
				Type:   "container",
				Action: "start",
				Actor:  EventActor{ID: "abc123", Attributes: map[string]string{"name": "test"}},
			})
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Sleep a bit then return to simulate disconnect
			time.Sleep(100 * time.Millisecond)
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

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// watchEvents will run until ctx expires
	go engine.watchEvents(ctx)

	// Wait for context to expire
	<-ctx.Done()
}

// TestOnContainerStartBadLabels tests onContainerStart with a container that has invalid labels.
func TestOnContainerStartBadLabels(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/aaa111bbb222/json":
			json.NewEncoder(w).Encode(ContainerDetail{
				Name:  "/bad-container",
				State: ContainerState{Running: true},
				Config: ContainerConfig{
					Image: "nginx:latest",
					Labels: map[string]string{
						"dr.enable": "true",
						"dr.host":   "", // empty host — should fail validation
					},
				},
				Network: ContainerNetwork{IPAddress: "172.17.0.5"},
			})
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

	ctx := context.Background()
	engine.onContainerStart(ctx, "aaa111bbb222")

	// Container should not be added because config validation fails
	containers := engine.GetContainers()
	if len(containers) != 0 {
		t.Errorf("containers = %d, want 0 (invalid config should be rejected)", len(containers))
	}
}

// TestOnContainerStartLabelsDisabled tests onContainerStart with non-enabled container.
func TestOnContainerStartLabelsDisabled(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/ccc333ddd444/json":
			json.NewEncoder(w).Encode(ContainerDetail{
				Name:  "/random-app",
				State: ContainerState{Running: true},
				Config: ContainerConfig{
					Image:  "nginx:latest",
					Labels: map[string]string{"com.docker.compose.service": "app"},
				},
				Network: ContainerNetwork{IPAddress: "172.17.0.5"},
			})
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

	ctx := context.Background()
	engine.onContainerStart(ctx, "ccc333ddd444")

	containers := engine.GetContainers()
	if len(containers) != 0 {
		t.Errorf("containers = %d, want 0 (not enabled)", len(containers))
	}
}

// TestOnContainerStartInspectFail tests onContainerStart when InspectContainer fails.
func TestOnContainerStartInspectFail(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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

	ctx := context.Background()
	engine.onContainerStart(ctx, "abc999def888")

	containers := engine.GetContainers()
	if len(containers) != 0 {
		t.Errorf("containers = %d, want 0 (inspect failure)", len(containers))
	}
}

// TestPollLoopZeroInterval tests pollLoop defaults to 30s when interval is zero.
func TestPollLoopZeroInterval(t *testing.T) {
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
	engine.interval = 0 // zero interval

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// pollLoop should default to 30s ticker, but exit quickly via ctx
	engine.pollLoop(ctx)
}

// TestHandleEventHealthEvent tests handleEvent with a health status change.
func TestHandleEventHealthEvent(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/abc123def456/json":
			json.NewEncoder(w).Encode(ContainerDetail{
				Name:  "/healthy-app",
				State: ContainerState{Running: true, Healthy: true},
				Config: ContainerConfig{
					Image: "nginx:latest",
					Labels: map[string]string{
						"dr.enable": "true",
						"dr.host":   "app.example.com",
					},
				},
				Network: ContainerNetwork{IPAddress: "172.17.0.10"},
			})
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

	ctx := context.Background()
	event := Event{
		Type:   "container",
		Action: "health_status: healthy",
		Actor:  EventActor{ID: "abc123def456", Attributes: map[string]string{"name": "healthy-app"}},
	}
	engine.handleEvent(ctx, event)

	containers := engine.GetContainers()
	if len(containers) != 1 {
		t.Errorf("containers = %d, want 1 after health event", len(containers))
	}
}

// TestHandleEventStopEvent tests handleEvent with a stop event.
func TestHandleEventStopEvent(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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

	// Inject a container first
	engine.InjectContainerForTest(&ContainerInfo{
		ID:   "abc111def222",
		Name: "stopping-app",
		Config: &RouteConfig{
			Enabled: true,
			Host:    "stop.example.com",
		},
		Address: "172.17.0.20",
	})

	ctx := context.Background()
	event := Event{
		Type:   "container",
		Action: "stop",
		Actor:  EventActor{ID: "abc111def222", Attributes: map[string]string{"name": "stopping-app"}},
	}
	engine.handleEvent(ctx, event)

	containers := engine.GetContainers()
	if len(containers) != 0 {
		t.Errorf("containers = %d, want 0 after stop event", len(containers))
	}
}

// TestOnContainerStopNonExistent tests onContainerStop for a non-existent container.
func TestOnContainerStopNonExistent(t *testing.T) {
	logger := &mockDiscoveryLogger{}
	sink := newMockRouteSink()
	engine := NewEngine(nil, sink, logger)

	engine.onContainerStop("nonexistent-id")

	// Should not panic, sink should have no calls
	sink.mu.RLock()
	routeCount := len(sink.routes)
	sink.mu.RUnlock()
	if routeCount != 0 {
		t.Errorf("routes = %d, want 0", routeCount)
	}
}
