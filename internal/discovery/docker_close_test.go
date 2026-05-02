package discovery

import (
	"testing"
)

func TestDockerClientClose(t *testing.T) {
	client, err := NewDockerClient("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}

	// Close should not panic and should return nil
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Double close should also be safe
	if err := client.Close(); err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

func TestDockerClientCloseDefault(t *testing.T) {
	// Test with default socket path
	client, err := NewDockerClient("")
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}
	if client.socketPath != "/var/run/docker.sock" {
		t.Errorf("socketPath = %s, want /var/run/docker.sock", client.socketPath)
	}
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
