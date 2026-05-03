package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/discovery"
	"github.com/DockRouter/dockrouter/internal/health"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/router"
	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

// TestAppStartWithTLSManager tests start() with TLS manager (HTTPS server path).
func TestAppStartWithTLSManager(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()
	challengeSolver := tlspkg.NewChallengeSolver()
	checker := health.NewChecker(10*time.Second, 5*time.Second)

	store := tlspkg.NewStore(t.TempDir())
	acmeClient := tlspkg.NewACMEClient("", "test@example.com")
	tlsManager := tlspkg.NewManager(store, acmeClient, challengeSolver, logger)

	app := &App{
		logger:          logger,
		config:          &config.Config{HTTPPort: 0, HTTPSPort: 0, AdminPort: 0, AccessLog: false, Admin: false, DefaultTLS: "off"},
		routeTable:      rt,
		challengeSolver: challengeSolver,
		healthChecker:   checker,
		tlsManager:      tlsManager,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	app.start(ctx)

	// Verify HTTPS server was created
	if app.httpsServer == nil {
		t.Error("httpsServer should be created when tlsManager is set")
	}
	if app.httpServer == nil {
		t.Error("httpServer should always be created")
	}
}

// TestAppStartWithAdminEnabled tests start() with admin server enabled.
func TestAppStartWithAdminEnabled(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()
	challengeSolver := tlspkg.NewChallengeSolver()
	checker := health.NewChecker(10*time.Second, 5*time.Second)

	app := &App{
		logger:          logger,
		config:          &config.Config{HTTPPort: 0, HTTPSPort: -1, AdminPort: 0, AdminBind: "127.0.0.1", AccessLog: false, Admin: true, DefaultTLS: "off"},
		routeTable:      rt,
		challengeSolver: challengeSolver,
		healthChecker:   checker,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	app.start(ctx)

	if app.adminServer == nil {
		t.Error("adminServer should be created when Admin is true")
	}
}

// TestAppStartWithDiscoveryEngine tests start() with discovery engine.
func TestAppStartWithDiscoveryEngine(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()
	challengeSolver := tlspkg.NewChallengeSolver()
	checker := health.NewChecker(10*time.Second, 5*time.Second)

	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}
	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	app := &App{
		logger:          logger,
		config:          &config.Config{HTTPPort: 0, HTTPSPort: -1, AdminPort: -1, AccessLog: false, Admin: false, DefaultTLS: "off"},
		routeTable:      rt,
		challengeSolver: challengeSolver,
		healthChecker:   checker,
		discoveryEngine: engine,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	app.start(ctx)
}

// TestHandleStatusWithDiscoveryEngine tests status endpoint with containers.
func TestHandleStatusWithDiscoveryEngine(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}
	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "abc123",
		Name:    "test-app",
		Address: "172.17.0.5:8080",
		Healthy: true,
		Config:  &discovery.RouteConfig{Enabled: true, Host: "test.com"},
	})

	app := &App{
		logger:          logger,
		config:          &config.Config{Version: "test", HTTPPort: 8080, HTTPSPort: 8443},
		routeTable:      router.NewTable(),
		startTime:       time.Now().Add(-time.Minute),
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	app.handleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&result)

	if result["containers"] != float64(1) {
		t.Errorf("containers = %v, want 1", result["containers"])
	}
}

// TestHandleRoutesWithBackend tests routes endpoint with backend pool.
func TestHandleRoutesWithBackend(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	pool := router.NewBackendPool(router.RoundRobin)
	pool.Add(&router.BackendTarget{
		Address:     "172.17.0.2:8080",
		ContainerID: "abc123def456",
		Weight:      1,
		Healthy:     true,
	})

	rt.Add(&router.Route{
		ID:         "abc123def4567890",
		Host:       "api.example.com",
		PathPrefix: "/api",
		Backend:    pool,
		TLS:        router.TLSConfig{Mode: "auto"},
	})

	app := &App{
		logger:     logger,
		routeTable: rt,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	rec := httptest.NewRecorder()
	app.handleRoutes(rec, req)

	var entries []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&entries)

	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}

	if entries[0]["backend"] != "172.17.0.2:8080" {
		t.Errorf("backend = %v", entries[0]["backend"])
	}
	if entries[0]["tls"] != true {
		t.Errorf("tls = %v, want true", entries[0]["tls"])
	}
	if entries[0]["healthy"] != true {
		t.Errorf("healthy = %v, want true", entries[0]["healthy"])
	}
}

// TestHandleRoutesNoBackend tests routes with nil backend.
func TestHandleRoutesNoBackend(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	rt.Add(&router.Route{
		ID:         "nobe456789012",
		Host:       "nobe.example.com",
		PathPrefix: "/",
		Backend:    nil,
	})

	app := &App{
		logger:     logger,
		routeTable: rt,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	rec := httptest.NewRecorder()
	app.handleRoutes(rec, req)

	var entries []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&entries)

	if entries[0]["backend"] != "-" {
		t.Errorf("backend with nil pool = %v, want -", entries[0]["backend"])
	}
}

// TestBuildHTTPHandlerACMEChallenge tests ACME challenge path in HTTP handler.
func TestBuildHTTPHandlerACMEChallenge(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "off"},
		challengeSolver: challengeSolver,
		routeTable:      router.NewTable(),
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(inner)

	// Path doesn't match any challenge token, passes through to inner handler
	req := httptest.NewRequest("GET", "/normal/path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestBuildHTTPHandlerBadRequest tests redirect with no matching route.
func TestBuildHTTPHandlerBadRequest(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "auto"},
		challengeSolver: challengeSolver,
		routeTable:      router.NewTable(), // empty table, no routes
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := app.buildHTTPHandler(inner)

	req := httptest.NewRequest("GET", "http://unknown.example.com/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// No matching route should return 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestBuildHTTPHandlerRedirectHostWithPort tests redirect with host:port format.
func TestBuildHTTPHandlerRedirectHostWithPort(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()
	rt := router.NewTable()
	rt.Add(&router.Route{Host: "example.com", PathPrefix: "/", ContainerID: "test", ContainerName: "test"})

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "auto"},
		challengeSolver: challengeSolver,
		routeTable:      rt,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := app.buildHTTPHandler(inner)

	req := httptest.NewRequest("GET", "http://example.com:8080/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "https://") {
		t.Errorf("Location = %s, should redirect to https", loc)
	}
}

// TestHandleCertificatesWithTLSManager tests certificates endpoint with manager.
func TestHandleCertificatesWithTLSManager(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	store := tlspkg.NewStore(t.TempDir())
	acmeClient := tlspkg.NewACMEClient("", "test@example.com")
	tlsManager := tlspkg.NewManager(store, acmeClient, tlspkg.NewChallengeSolver(), logger)

	app := &App{
		logger:     logger,
		tlsManager: tlsManager,
	}

	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	rec := httptest.NewRecorder()
	app.handleCertificates(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}

	var entries []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&entries)

	// No certificates loaded yet, should return empty array
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

// TestShutdownHTTPSOnly tests shutdown with only HTTPS server.
func TestShutdownHTTPSOnly(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	ln, _ := newListener()
	httpsServer := &http.Server{Handler: http.DefaultServeMux}
	go httpsServer.Serve(ln)

	app := &App{
		logger:      logger,
		httpServer:  nil,
		httpsServer: httpsServer,
		adminServer: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	app.shutdown(ctx)
}

func newListener() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}
