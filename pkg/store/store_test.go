package store

import (
	"context"
	"os"
	"testing"

	"agent-memory/pkg/api"
)

func setupStore(t *testing.T) (*SQLStore, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	s, err := NewStore(dir)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to create store: %v", err)
	}
	return s, func() {
		s.Close()
		os.RemoveAll(dir)
	}
}

func TestSearchWindowed(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()

	ctx := context.Background()

	entries := []api.LogEntry{
		{SessionID: "test", Role: "a", Content: "Plan: setting up PostgreSQL database", Timestamp: 100},
		{SessionID: "test", Role: "b", Content: "Decision: using connection pooling via PgBouncer", Timestamp: 200},
		{SessionID: "test", Role: "c", Content: "Code: implementing the schema with tables", Timestamp: 300},
		{SessionID: "test", Role: "d", Content: "API routes for users endpoint", Timestamp: 400},
		{SessionID: "test", Role: "e", Content: "Deploy: docker compose setup", Timestamp: 500},
	}
	for _, e := range entries {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("failed to append: %v", err)
		}
	}

	t.Run("context_zero_returns_only_match", func(t *testing.T) {
		results, err := store.Search(ctx, "test", "PostgreSQL", 0)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result with context=0, got %d", len(results))
		}
		if results[0].Role != "a" {
			t.Errorf("expected role 'a', got '%s'", results[0].Role)
		}
	})

	t.Run("context_two_returns_window_around_match", func(t *testing.T) {
		results, err := store.Search(ctx, "test", "PostgreSQL", 2)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		// Should include entries at positions 1, 2, 3 (match at 1, context of 2 after)
		if len(results) != 3 {
			for _, r := range results {
				t.Logf("  [%d] role=%s content=%s", r.ID, r.Role, r.Content)
			}
			t.Fatalf("expected 3 results with context=2, got %d", len(results))
		}
	})

	t.Run("context_large_returns_all", func(t *testing.T) {
		results, err := store.Search(ctx, "test", "PostgreSQL", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if len(results) != 5 {
			t.Fatalf("expected 5 results with context=10, got %d", len(results))
		}
	})
}
