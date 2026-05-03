package admin

import (
	"testing"
	"time"
)

// TestSSEHubHandlerDoneDuringRegister tests that Handler returns when hub stops during registration.
func TestSSEHubHandlerDoneDuringRegister(t *testing.T) {
	hub := NewSSEHub()

	// Stop the hub before Run starts — done channel will be closed
	close(hub.done)

	// Handler would return immediately since done is already closed
	// Test the select logic directly by sending to register when done is closed

	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	select {
	case hub.register <- client:
		t.Error("register should not succeed when done is closed")
	case <-hub.done:
		// Expected: done channel was already closed
	}
}

// TestSSEHubClientChannelClosedOnUnregister tests that reading from a closed client channel returns false.
func TestSSEHubClientChannelClosedOnUnregister(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client
	time.Sleep(20 * time.Millisecond)

	// Unregister closes the channel
	hub.unregister <- client
	time.Sleep(50 * time.Millisecond)

	// Reading from closed channel should return zero value and false
	_, ok := <-client.ch
	if ok {
		t.Error("client channel should be closed after unregister")
	}
}

// TestSSEHubSendWhenRunning tests Send delivers event to a registered client.
func TestSSEHubSendWhenRunning(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client
	time.Sleep(20 * time.Millisecond)

	hub.Send(Event{Type: "test", Data: "hello"})
	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-client.ch:
		if event.Type != "test" {
			t.Errorf("event type = %q, want test", event.Type)
		}
	default:
		t.Error("client should have received the event")
	}
}

// TestSSEHubDroppedIncrement tests that dropped counter increments on full broadcast.
func TestSSEHubDroppedIncrement(t *testing.T) {
	hub := NewSSEHub()
	// Don't start Run — broadcast channel has capacity 100 but nobody drains it
	// Fill it up
	for i := 0; i < 100; i++ {
		hub.broadcast <- Event{Type: "fill", Data: i}
	}

	// Next send should be dropped
	before := hub.dropped.Load()
	hub.Send(Event{Type: "overflow"})
	after := hub.dropped.Load()

	if after <= before {
		t.Errorf("dropped: before=%d after=%d, should increase", before, after)
	}
}

// TestSSEHubMultipleRegisterUnregister tests rapid register/unregister cycles.
func TestSSEHubMultipleRegisterUnregister(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	for i := 0; i < 10; i++ {
		client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
		hub.register <- client
		time.Sleep(5 * time.Millisecond)

		hub.unregister <- client
		time.Sleep(5 * time.Millisecond)
	}

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("clients = %d, want 0 after all unregistered", count)
	}
}

// TestSSEHubBroadcastToEmptyClients tests broadcasting when no clients connected.
func TestSSEHubBroadcastToEmptyClients(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	defer hub.Stop()

	// Send to empty hub — should not panic
	hub.Send(Event{Type: "empty", Data: "none"})
	time.Sleep(50 * time.Millisecond)
}
