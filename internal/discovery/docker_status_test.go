package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestDoStreamRequestBadStatus tests doStreamRequest when server returns non-2xx status.
func TestDoStreamRequestBadStatus(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("service unavailable"))
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	_, err = client.doStreamRequest(context.Background(), http.MethodGet, "/events")
	if err == nil {
		t.Fatal("doStreamRequest should fail with 503 status")
	}
}

// TestListNetworksParseError tests ListNetworks with invalid JSON response.
func TestListNetworksParseError(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	_, err = client.ListNetworks(context.Background())
	if err == nil {
		t.Fatal("ListNetworks should fail with invalid JSON")
	}
}

// TestListNetworksSuccessWithData tests ListNetworks with actual network data.
func TestListNetworksSuccessWithData(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		networks := []Network{
			{
				ID:     "net1",
				Name:   "bridge",
				Driver: "bridge",
				Scope:  "local",
				Subnets: []Subnet{
					{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"},
				},
			},
			{
				ID:     "net2",
				Name:   "host",
				Driver: "host",
				Scope:  "local",
			},
		}
		json.NewEncoder(w).Encode(networks)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	networks, err := client.ListNetworks(context.Background())
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(networks) != 2 {
		t.Errorf("networks count = %d, want 2", len(networks))
	}
	if networks[0].Name != "bridge" {
		t.Errorf("networks[0].Name = %q, want bridge", networks[0].Name)
	}
	if len(networks[0].Subnets) != 1 {
		t.Errorf("networks[0].Subnets count = %d, want 1", len(networks[0].Subnets))
	}
}
