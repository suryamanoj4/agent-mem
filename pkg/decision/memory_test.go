package decision

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMemoryDecisionStore_Store(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	d := Decision{
		SessionID:    "test-session",
		AgentID:      "agent-1",
		AuthorType:   AuthorTypeAgent,
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "Using PostgreSQL for persistence"},
		Timestamp:    time.Now(),
	}

	if err := store.Store(ctx, d); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
}

func TestMemoryDecisionStore_Search(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "agent-1",
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "Using PostgreSQL"},
		Timestamp:    time.Now(),
	})
	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "agent-2",
		DecisionType: DecisionTypePlan,
		Content:      DecisionContent{Summary: "Phase 1 complete"},
		Timestamp:    time.Now(),
	})

	results, err := store.Search(ctx, SearchRequest{SessionID: "s1", Query: "PostgreSQL", Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content.Summary != "Using PostgreSQL" {
		t.Errorf("summary mismatch: got %q", results[0].Content.Summary)
	}
}

func TestMemoryDecisionStore_GetByType(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "a1",
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "arch decision"},
		Timestamp:    time.Now(),
	})
	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "a1",
		DecisionType: DecisionTypePlan,
		Content:      DecisionContent{Summary: "plan decision"},
		Timestamp:    time.Now(),
	})

	results, err := store.GetByType(ctx, "s1", DecisionTypeArchitecture)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 architecture decision, got %d", len(results))
	}
}

func TestMemoryDecisionStore_ListSessions(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d1"}, Timestamp: time.Now()})
	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d2"}, Timestamp: time.Now()})
	store.Store(ctx, Decision{SessionID: "s2", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d3"}, Timestamp: time.Now()})

	sessions, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	sessionsMap := make(map[string]int)
	for _, s := range sessions {
		sessionsMap[s.SessionID] = s.TotalDecisions
	}

	if sessionsMap["s1"] != 2 {
		t.Errorf("s1 expected 2 decisions, got %d", sessionsMap["s1"])
	}
	if sessionsMap["s2"] != 1 {
		t.Errorf("s2 expected 1 decision, got %d", sessionsMap["s2"])
	}
}

func TestMemoryDecisionStore_Export(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "agent-1",
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "arch decision", Diff: "--- a/foo\n+++ b/foo"},
		Timestamp:    time.Now(),
	})

	var sb strings.Builder
	if err := store.Export(ctx, "s1", &sb); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := sb.String()
	if !strings.Contains(output, "agent-1") {
		t.Errorf("export missing agent-1: %s", output)
	}
	if !strings.Contains(output, "arch decision") {
		t.Errorf("export missing summary: %s", output)
	}
}

func TestMemoryDecisionStore_PruneEntries(t *testing.T) {
	store := NewMemoryDecisionStore()
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now()

	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "old archived"}, Timestamp: old, Archived: true})
	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "recent archived"}, Timestamp: recent, Archived: true})
	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "active entry"}, Timestamp: recent})

	count, err := store.PruneEntries(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("PruneEntries failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pruned, got %d", count)
	}

	results, _ := store.ListSessions(ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 session after prune, got %d", len(results))
	}
	if results[0].TotalDecisions != 1 {
		t.Errorf("expected 1 remaining active decision, got %d", results[0].TotalDecisions)
	}
}