package discovery

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// unixMockServer creates a mock Docker API server listening on a Unix socket
type unixMockServer struct {
	server   *http.Server
	listener net.Listener
	socket   string
}

func newUnixMockServer(handler http.Handler) (*unixMockServer, error) {
	tmpDir := os.TempDir()
	socket := filepath.Join(tmpDir, "docker-mock-"+generateTestSuffix()+".sock")

	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}

	server := &http.Server{Handler: handler}
	go server.Serve(listener)

	return &unixMockServer{
		server:   server,
		listener: listener,
		socket:   socket,
	}, nil
}

func (m *unixMockServer) Close() {
	m.server.Close()
	os.Remove(m.socket)
}

func generateTestSuffix() string {
	return time.Now().Format("150405.000")
}

// dockerAPIHandler returns a mock Docker API handler with standard container data
func dockerAPIHandler() http.Handler {
	mux := http.NewServeMux()

	// GET /containers/json - list containers
	mux.HandleFunc("/v1.53/containers/json", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "running" || r.URL.Query().Get("all") == "true" {
			containers := []Container{
				{
					ID:     "abc123def456",
					Names:  []string{"/test-app"},
					Image:  "nginx:latest",
					State:  "running",
					Status: "Up 2 hours",
					Labels: map[string]string{
						"dr.enable": "true",
						"dr.host":   "test.example.com",
						"dr.port":   "8080",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(containers)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Container{})
		}
	})

	// GET /containers/{id}/json - inspect container
	mux.HandleFunc("/v1.53/containers/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Extract container ID from path: /v1.53/containers/{id}/json
		parts := strings.SplitN(path, "/", 5)
		if len(parts) < 5 {
			http.NotFound(w, r)
			return
		}

		containerID := parts[4]
		if strings.HasSuffix(containerID, "/json") {
			containerID = strings.TrimSuffix(containerID, "/json")
		}

		detail := ContainerDetail{
			ID:   containerID,
			Name: "/test-app",
			State: ContainerState{
				Status:  "running",
				Running: true,
				Healthy: true,
			},
			Config: ContainerConfig{
				Labels: map[string]string{
					"dr.enable": "true",
					"dr.host":   "test.example.com",
					"dr.port":   "8080",
				},
				Image: "nginx:latest",
			},
			Network: ContainerNetwork{
				IPAddress: "172.17.0.2",
				Networks: map[string]NetworkInfo{
					"bridge": {
						IPAddress: "172.17.0.2",
						Gateway:   "172.17.0.1",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})

	// GET /events - event stream (returns empty then closes)
	mux.HandleFunc("/v1.53/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Keep connection open briefly then close
		time.Sleep(100 * time.Millisecond)
	})

	// GET /_ping
	mux.HandleFunc("/v1.53/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// GET /networks
	mux.HandleFunc("/v1.53/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Network{
			{
				Name:   "bridge",
				Driver: "bridge",
				Subnets: []Subnet{
					{Subnet: "172.17.0.0/16"},
				},
			},
		})
	})

	return mux
}

func TestEngineSyncWithUnixMock(t *testing.T) {
	mock, err := newUnixMockServer(dockerAPIHandler())
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	sink := newMockRouteSink()
	logger := &mockLogger{}
	engine := NewEngine(client, sink, logger)
	engine.interval = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = engine.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}

	// Should have discovered the test container
	containers := engine.GetContainers()
	if len(containers) != 1 {
		t.Fatalf("container count = %d, want 1", len(containers))
	}

	c := containers[0]
	if c.Name != "test-app" {
		t.Errorf("name = %s, want test-app", c.Name)
	}
	if c.Config.Host != "test.example.com" {
		t.Errorf("host = %s, want test.example.com", c.Config.Host)
	}

	// Route sink should have received the route
	sink.mu.Lock()
	routeCount := len(sink.routes)
	sink.mu.Unlock()
	if routeCount != 1 {
		t.Errorf("route sink count = %d, want 1", routeCount)
	}
}

func TestEngineSyncRemovesStaleWithUnixMock(t *testing.T) {
	handler := dockerAPIHandler()
	mock, err := newUnixMockServer(handler)
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	sink := newMockRouteSink()
	logger := &mockLogger{}
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()

	// First sync
	engine.Sync(ctx)

	// Add a stale container manually
	engine.mu.Lock()
	engine.containers["stale-container"] = &ContainerInfo{
		ID:   "stale-container",
		Name: "stale",
	}
	engine.mu.Unlock()

	// Second sync should remove stale container
	err = engine.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}

	containers := engine.GetContainers()
	for _, c := range containers {
		if c.ID == "stale-container" {
			t.Error("stale container should have been removed")
		}
	}

	// Check that RemoveRoute was called for stale container
	sink.mu.Lock()
	var foundStale bool
	for _, id := range sink.removed {
		if id == "stale-container" {
			foundStale = true
		}
	}
	sink.mu.Unlock()
	if !foundStale {
		t.Error("RemoveRoute should have been called for stale container")
	}
}

func TestEngineOnContainerStartWithUnixMock(t *testing.T) {
	mock, err := newUnixMockServer(dockerAPIHandler())
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	sink := newMockRouteSink()
	logger := &mockLogger{}
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()

	// Trigger onContainerStart
	engine.onContainerStart(ctx, "abc123def456")

	// Container should be in the map
	c := engine.GetContainer("abc123def456")
	if c == nil {
		t.Fatal("container should be registered")
	}
	if c.Name != "test-app" {
		t.Errorf("name = %s, want test-app", c.Name)
	}
	if c.Config.Host != "test.example.com" {
		t.Errorf("host = %s, want test.example.com", c.Config.Host)
	}

	// Route sink should have received the route
	sink.mu.Lock()
	_, exists := sink.routes["abc123def456"]
	sink.mu.Unlock()
	if !exists {
		t.Error("route should have been added to sink")
	}
}

func TestEngineOnContainerStartDisabled(t *testing.T) {
	// Create handler that returns a container without dr.enable
	handler := http.NewServeMux()
	handler.HandleFunc("/v1.53/containers/", func(w http.ResponseWriter, r *http.Request) {
		detail := ContainerDetail{
			ID:   "disabled123",
			Name: "/disabled-app",
			State: ContainerState{
				Status:  "running",
				Running: true,
			},
			Config: ContainerConfig{
				Labels: map[string]string{},
				Image:  "nginx:latest",
			},
			Network: ContainerNetwork{
				IPAddress: "172.17.0.3",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})

	mock, err := newUnixMockServer(handler)
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	sink := newMockRouteSink()
	logger := &mockLogger{}
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()
	engine.onContainerStart(ctx, "disabled123")

	// Container should NOT be in the map (dr.enable not set)
	c := engine.GetContainer("disabled123")
	if c != nil {
		t.Error("disabled container should not be registered")
	}
}

func TestEngineOnContainerStartInvalidConfig(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/v1.53/containers/", func(w http.ResponseWriter, r *http.Request) {
		detail := ContainerDetail{
			ID:   "invalid123",
			Name: "/invalid-app",
			State: ContainerState{
				Status:  "running",
				Running: true,
			},
			Config: ContainerConfig{
				Labels: map[string]string{
					"dr.enable": "true",
					// Missing dr.host - invalid config
				},
				Image: "nginx:latest",
			},
			Network: ContainerNetwork{
				IPAddress: "172.17.0.4",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})

	mock, err := newUnixMockServer(handler)
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	sink := newMockRouteSink()
	logger := &mockLogger{}
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()
	engine.onContainerStart(ctx, "invalid123")

	// Container should NOT be registered (missing host)
	c := engine.GetContainer("invalid123")
	if c != nil {
		t.Error("container with invalid config should not be registered")
	}
}
