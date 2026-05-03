package discovery

import (
	"context"
	"testing"
	"time"
)

// TestChangedBothNilConfigs tests Changed when both configs are nil.
func TestChangedBothNilConfigs(t *testing.T) {
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  nil,
	}
	other := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  nil,
	}
	if ci.Changed(other) {
		t.Error("should not detect change when both configs are nil")
	}
}

// TestChangedOneNilConfig tests Changed when one config is nil.
func TestChangedOneNilConfig(t *testing.T) {
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	other := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  nil,
	}
	if !ci.Changed(other) {
		t.Error("should detect change when one config is nil and other is not")
	}

	// Reverse case
	if !other.Changed(ci) {
		t.Error("should detect change in reverse direction too")
	}
}

// TestChangedReceiverNilConfig tests Changed when receiver has nil config.
func TestChangedReceiverNilConfig(t *testing.T) {
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  nil,
	}
	other := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	if !ci.Changed(other) {
		t.Error("should detect change when receiver config is nil")
	}
}

// TestChangedAddressDifference tests Changed with different addresses only.
func TestChangedAddressDifference(t *testing.T) {
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	other := &ContainerInfo{
		Address: "10.0.0.2:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	if !ci.Changed(other) {
		t.Error("should detect address change")
	}
}

// TestChangedHealthDifference tests Changed with different healthy status only.
func TestChangedHealthDifference(t *testing.T) {
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: false,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	other := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}
	if !ci.Changed(other) {
		t.Error("should detect health change")
	}
}

// TestChangedIdentical tests Changed with identical ContainerInfo.
func TestChangedIdentical(t *testing.T) {
	cfg := &RouteConfig{Host: "test.com", Path: "/api"}
	ci := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  cfg,
	}
	other := &ContainerInfo{
		Address: "10.0.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/api"},
	}
	if ci.Changed(other) {
		t.Error("should not detect change when identical")
	}
}

// TestEngineStartAlreadyRunning tests that Start returns nil when already running.
func TestEngineStartAlreadyRunningGuard(t *testing.T) {
	logger := &mockDiscoveryLogger{}
	sink := newMockRouteSink()
	client, _ := NewDockerClient("/nonexistent.sock")

	engine := &Engine{
		client:     client,
		events:     NewEventStream(client),
		poller:     NewPoller(client, 10*time.Second),
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
		running:    true, // Already running
	}

	ctx := context.Background()
	err := engine.Start(ctx)
	if err != nil {
		t.Errorf("Start should return nil when already running, got %v", err)
	}
}
