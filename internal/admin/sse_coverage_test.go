package admin

import (
	"testing"
	"time"
)

// TestSSEHubSendChannelFull tests that Send drops events when broadcast channel is full.
func TestSSEHubSendChannelFull(t *testing.T) {
	hub := NewSSEHub()

	// Fill broadcast channel (capacity 100)
	for i := 0; i < 100; i++ {
		hub.broadcast <- Event{Type: "fill", Data: i}
	}

	// Next Send should hit the default case and increment dropped
	hub.Send(Event{Type: "overflow", Data: "extra"})

	dropped := hub.dropped.Load()
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

// TestSSEHubSendMultipleOverflow tests multiple overflows increment counter.
func TestSSEHubSendMultipleOverflow(t *testing.T) {
	hub := NewSSEHub()

	// Fill broadcast channel
	for i := 0; i < 100; i++ {
		hub.broadcast <- Event{Type: "fill", Data: i}
	}

	// Send multiple overflows
	for i := 0; i < 5; i++ {
		hub.Send(Event{Type: "overflow", Data: i})
	}

	dropped := hub.dropped.Load()
	if dropped != 5 {
		t.Errorf("dropped = %d, want 5", dropped)
	}
}

// TestSSEHubSendAfterStop tests Send on closed hub.
func TestSSEHubSendAfterStopClosed(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// Send after stop - should not panic
	hub.Send(Event{Type: "after-stop", Data: "test"})
}

// TestSSEHubDroppedCounterStarts tests that dropped counter starts at 0.
func TestSSEHubDroppedCounterStarts(t *testing.T) {
	hub := NewSSEHub()
	if hub.dropped.Load() != 0 {
		t.Error("dropped counter should start at 0")
	}
}
