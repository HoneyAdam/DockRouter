package router

import (
	"testing"
)

// --- RadixTree edge case tests ---

func TestRadixTreeInsertEmptyPath(t *testing.T) {
	tree := NewRadixTree()

	// Insert empty path - should be normalized to "/"
	tree.Insert("", &Route{ID: "empty"})

	route := tree.Match("/")
	if route == nil || route.ID != "empty" {
		t.Error("Empty path should be normalized to root")
	}
}

func TestRadixTreeInsertDuplicatePaths(t *testing.T) {
	tree := NewRadixTree()

	// Insert same path multiple times
	tree.Insert("/api", &Route{ID: "api-v1"})
	tree.Insert("/api", &Route{ID: "api-v2"})
	tree.Insert("/api", &Route{ID: "api-v3"})

	route := tree.Match("/api")
	if route == nil || route.ID != "api-v3" {
		t.Errorf("Route should be last inserted version, got %v", route)
	}
}

func TestRadixTreeMatchEmptyTree(t *testing.T) {
	tree := NewRadixTree()

	// Match on empty tree should return nil
	route := tree.Match("/api")
	if route != nil {
		t.Error("Match on empty tree should return nil")
	}
}

// --- Common prefix edge cases ---

func TestCommonPrefixEmptyStrings(t *testing.T) {
	result := commonPrefix("", "")
	if result != "" {
		t.Errorf("commonPrefix(\"\", \"\") = %q, want empty", result)
	}
}

func TestCommonPrefixFirstEmpty(t *testing.T) {
	result := commonPrefix("", "/api")
	if result != "" {
		t.Errorf("commonPrefix(\"\", \"/api\") = %q, want empty", result)
	}
}

func TestCommonPrefixSecondEmpty(t *testing.T) {
	result := commonPrefix("/api", "")
	if result != "" {
		t.Errorf("commonPrefix(\"/api\", \"\") = %q, want empty", result)
	}
}

func TestCommonPrefixFullMatch(t *testing.T) {
	result := commonPrefix("/api/v1", "/api/v1")
	if result != "/api/v1" {
		t.Errorf("commonPrefix(\"/api/v1\", \"/api/v1\") = %q", result)
	}
}

func TestCommonPrefixPartialMatch(t *testing.T) {
	result := commonPrefix("/api/v1/users", "/api/v1/orders")
	if result != "/api/v1/" {
		t.Errorf("commonPrefix(\"/api/v1/users\", \"/api/v1/orders\") = %q", result)
	}
}

// --- Normalize path edge cases ---

func TestNormalizePathMultipleSlashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"//", "/"},
		{"/api//v1", "/api//v1"}, // double slash in middle is preserved
		{"/api/", "/api"},
		{"api", "/api"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Table edge cases ---

func TestTableAdd(t *testing.T) {
	table := NewTable()

	route := &Route{
		ID:          "test-route",
		Host:        "example.com",
		PathPrefix:  "/api",
		ContainerID: "container-1",
	}

	table.Add(route)

	// Verify route was added
	routes := table.List()
	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}
}

func TestTableRemove(t *testing.T) {
	table := NewTable()

	route := &Route{
		ID:          "test-route",
		Host:        "example.com",
		PathPrefix:  "/api",
		ContainerID: "container-1",
	}

	table.Add(route)
	table.Remove("test-route")

	// Verify route was removed
	routes := table.List()
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes, got %d", len(routes))
	}
}

func TestTableMatchNotFound(t *testing.T) {
	table := NewTable()

	// Match on empty table should return nil
	route := table.Match("example.com", "/api")
	if route != nil {
		t.Error("Match on empty table should return nil")
	}
}

func TestTableMatchWithWildcard(t *testing.T) {
	table := NewTable()

	table.Add(&Route{ID: "wildcard", Host: "*.example.com", PathPrefix: "/api"})

	// Should match subdomain
	route := table.Match("app.example.com", "/api")
	if route == nil || route.ID != "wildcard" {
		t.Error("Should match wildcard domain")
	}
}
