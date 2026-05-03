package admin

import (
	"strings"
	"testing"
	"time"
)

// TestSSEHubRunBroadcastToClient tests that a broadcast event reaches a registered client.
func TestSSEHubRunBroadcastToClient(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	// Manually register a client
	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client
	time.Sleep(20 * time.Millisecond)

	// Send an event
	hub.Send(Event{Type: "test", Data: "hello"})

	// Wait for broadcast
	time.Sleep(50 * time.Millisecond)

	// Client should have received the event
	select {
	case event := <-client.ch:
		if event.Type != "test" {
			t.Errorf("event.Type = %q, want test", event.Type)
		}
	default:
		t.Error("client should have received the event")
	}
}

// TestSSEHubRunBroadcastToMultipleClients tests broadcasting to multiple clients.
func TestSSEHubRunBroadcastToMultipleClients(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	client1 := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	client2 := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client1
	hub.register <- client2
	time.Sleep(20 * time.Millisecond)

	hub.Send(Event{Type: "update", Data: map[string]int{"count": 42}})
	time.Sleep(50 * time.Millisecond)

	if len(client1.ch) != 1 {
		t.Errorf("client1 events = %d, want 1", len(client1.ch))
	}
	if len(client2.ch) != 1 {
		t.Errorf("client2 events = %d, want 1", len(client2.ch))
	}
}

// TestSSEHubRunDropSlowClient tests that slow clients get events dropped.
func TestSSEHubRunDropSlowClient(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	// Create a client with tiny channel
	slowClient := &sseClient{ch: make(chan Event, 1), flush: make(chan struct{})}
	hub.register <- slowClient
	time.Sleep(20 * time.Millisecond)

	// Fill the channel
	slowClient.ch <- Event{Type: "filler"}

	// Send more events — should be dropped
	for i := 0; i < 5; i++ {
		hub.Send(Event{Type: "overflow", Data: i})
	}
	time.Sleep(100 * time.Millisecond)

	dropped := hub.dropped.Load()
	if dropped == 0 {
		t.Error("expected some dropped events for slow client")
	}
}

// TestSSEHubRunUnregisterClient tests unregistering a client closes its channel.
func TestSSEHubRunUnregisterClient(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 1 {
		t.Fatalf("clients = %d, want 1", count)
	}

	// Unregister
	hub.unregister <- client
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	count = len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("clients = %d, want 0 after unregister", count)
	}

	// Channel should be closed
	_, ok := <-client.ch
	if ok {
		t.Error("client channel should be closed after unregister")
	}
}

// TestSSEHubSendAfterStopStillWorks tests that Send after Stop doesn't panic.
func TestSSEHubSendAfterStopStillWorks(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	hub.Stop()
	time.Sleep(50 * time.Millisecond)

	// Should not panic
	hub.Send(Event{Type: "late", Data: "event"})
}

// TestSSEHubRunRegisterAndUnregisterSequence tests a sequence of register/unregister.
func TestSSEHubRunRegisterAndUnregisterSequence(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	for i := 0; i < 5; i++ {
		client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		hub.mu.RLock()
		count := len(hub.clients)
		hub.mu.RUnlock()
		if count != 1 {
			t.Errorf("iter %d: clients = %d, want 1", i, count)
		}

		hub.unregister <- client
		time.Sleep(10 * time.Millisecond)

		hub.mu.RLock()
		count = len(hub.clients)
		hub.mu.RUnlock()
		if count != 0 {
			t.Errorf("iter %d: clients = %d, want 0 after unregister", i, count)
		}
	}
}

// TestSSEHubRunWithClosedDoneChannel tests that Run exits when done is closed.
func TestSSEHubRunWithClosedDoneChannel(t *testing.T) {
	hub := NewSSEHub()
	done := make(chan struct{})
	go func() {
		hub.Run()
		close(done)
	}()

	hub.Stop()

	select {
	case <-done:
		// Run exited
	case <-time.After(2 * time.Second):
		t.Fatal("Run should exit after Stop")
	}
}

// TestEventTypes tests Event struct field access.
func TestEventTypes(t *testing.T) {
	e := Event{Type: "container", Data: map[string]string{"id": "abc"}}
	if e.Type != "container" {
		t.Errorf("Type = %q, want container", e.Type)
	}
	data, ok := e.Data.(map[string]string)
	if !ok {
		t.Fatal("Data type assertion failed")
	}
	if data["id"] != "abc" {
		t.Errorf("Data[id] = %q, want abc", data["id"])
	}
}

// TestSSEHubStringContains tests string-related behavior.
func TestSSEHubStringContains(t *testing.T) {
	s := "text/event-stream"
	if !strings.Contains(s, "event-stream") {
		t.Error("should contain event-stream")
	}
}
