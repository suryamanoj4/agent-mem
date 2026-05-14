package service

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"agent-memory/pkg/api"
	"agent-memory/pkg/privacy"
	"agent-memory/pkg/store"
)

func TestMemoryService_EndToEnd(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agent-mem-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s, err := store.NewStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	svc := NewMemoryService(s)
	defer svc.Close()

	ctx := context.Background()
	sessionID := "test-session"
	role := "test-agent"
	content := "This is a test decision."

	sess, err := svc.Connect(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	err = sess.Log(ctx, role, content)
	if err != nil {
		t.Fatalf("failed to log entry: %v", err)
	}

	results, err := sess.Ask(ctx, "test", 5)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected search results, got 0")
	}
}

func TestMemoryService_Locking(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-lock-test-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	svc := NewMemoryService(s)
	defer svc.Close()

	ctx := context.Background()
	sess, _ := svc.Connect(ctx, "lock-session")
	path := "src/main.go"

	unlock, err := sess.Lock(ctx, path, "test-agent", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	locked, owner, _ := sess.GetLockStatus(ctx, path)
	if !locked || owner == "" {
		t.Errorf("expected file to be locked, got locked=%v, owner=%s", locked, owner)
	}

	_, err = sess.Lock(ctx, path, "test-agent-2", 1*time.Hour)
	if err == nil {
		t.Errorf("expected error when acquiring already locked file, got nil")
	}

	unlock()

	locked, _, _ = sess.GetLockStatus(ctx, path)
	if locked {
		t.Errorf("expected file to be unlocked after release")
	}
}

func TestMemoryService_CrossProcessLocking(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-cross-lock-*")
	defer os.RemoveAll(tempDir)

	// Simulate two separate agent-mem processes sharing the same DB
	s1, _ := store.NewStore(tempDir)
	svc1 := NewMemoryService(s1)
	defer svc1.Close()

	s2, _ := store.NewStore(tempDir)
	svc2 := NewMemoryService(s2)
	defer svc2.Close()

	ctx := context.Background()
	sess1, _ := svc1.Connect(ctx, "shared-session")
	sess2, _ := svc2.Connect(ctx, "shared-session")

	path := "src/main.go"

	// Agent A locks
	_, err := sess1.Lock(ctx, path, "agent-a", 1*time.Hour)
	if err != nil {
		t.Fatalf("agent-a failed to acquire lock: %v", err)
	}

	// Agent B should see it locked
	locked, owner, _ := sess2.GetLockStatus(ctx, path)
	if !locked {
		t.Errorf("agent-b should see file as locked")
	}
	if owner != "agent-a" {
		t.Errorf("expected owner to be agent-a, got %s", owner)
	}

	// Agent B should NOT be able to acquire it
	_, err = sess2.Lock(ctx, path, "agent-b", 1*time.Hour)
	if err == nil {
		t.Errorf("agent-b should not be able to lock a file held by agent-a")
	}
}

func TestCompact(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-compact-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	svc := NewMemoryService(s)
	defer svc.Close()

	ctx := context.Background()
	sess, _ := svc.Connect(ctx, "compact-session")

	// Log two entries
	sess.Log(ctx, "agent-a", "First decision about auth")
	sess.Log(ctx, "agent-b", "Second decision about database")

	// Both should be searchable
	results, _ := sess.Ask(ctx, "decision", 5)
	if len(results) != 2 {
		t.Errorf("expected 2 results before compact, got %d", len(results))
	}

	// Compact
	count, err := sess.Compact(ctx)
	if err != nil {
		t.Fatalf("failed to compact: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 entries archived, got %d", count)
	}

	// Search should now return nothing (entries are archived)
	results, _ = sess.Ask(ctx, "decision", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results after compact, got %d", len(results))
	}

	// New entries should be searchable
	sess.Log(ctx, "agent-c", "Third decision about caching")
	results, _ = sess.Ask(ctx, "decision", 5)
	if len(results) != 1 {
		t.Errorf("expected 1 result after new entry, got %d", len(results))
	}
}

func TestExport(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agent-mem-export-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s, err := store.NewStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	sessionID := "test-session"

	entries := []api.LogEntry{
		{SessionID: sessionID, Role: "agent-a", Content: "First decision", Timestamp: 1000},
		{SessionID: sessionID, Role: "agent-b", Content: "Second decision", Timestamp: 2000},
	}
	for _, e := range entries {
		if err := s.Append(ctx, e); err != nil {
			t.Fatalf("failed to append entry: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := s.Export(ctx, sessionID, &buf); err != nil {
		t.Fatalf("failed to export: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "agent-a") {
		t.Errorf("export missing role 'agent-a'")
	}
	if !strings.Contains(output, "First decision") {
		t.Errorf("export missing content 'First decision'")
	}
	if !strings.Contains(output, "agent-b") {
		t.Errorf("export missing role 'agent-b'")
	}
	if !strings.Contains(output, "Second decision") {
		t.Errorf("export missing content 'Second decision'")
	}
	if !strings.Contains(output, "---") {
		t.Errorf("export missing markdown separator")
	}
}

func TestListSessions(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-list-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	svc := NewMemoryService(s)
	defer svc.Close()

	ctx := context.Background()
	sessA, _ := svc.Connect(ctx, "session-a")
	sessB, _ := svc.Connect(ctx, "session-b")

	sessA.Log(ctx, "agent", "decision in A")
	sessA.Log(ctx, "agent", "another in A")
	sessB.Log(ctx, "agent", "decision in B")

	sessions, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	for _, info := range sessions {
		switch info.ID {
		case "session-a":
			if info.EntryCount != 2 {
				t.Errorf("session-a: expected 2 total entries, got %d", info.EntryCount)
			}
			if info.ActiveCount != 2 {
				t.Errorf("session-a: expected 2 active entries, got %d", info.ActiveCount)
			}
		case "session-b":
			if info.EntryCount != 1 {
				t.Errorf("session-b: expected 1 entry, got %d", info.EntryCount)
			}
		}
	}
}

func TestPruneEntries(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-prune-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	defer s.Close()

	ctx := context.Background()

	// Insert old archived entry
	s.Append(ctx, api.LogEntry{
		SessionID: "test", Role: "agent", Content: "old archived",
		Timestamp: 100, // very old
	})
	s.ArchiveEntries(ctx, "test")

	// Insert new active entry
	s.Append(ctx, api.LogEntry{
		SessionID: "test", Role: "agent", Content: "new entry",
		Timestamp: 2000000000,
	})

	// Prune with a cutoff that includes the old entry
	count, err := s.PruneEntries(ctx, 24*30*time.Hour) // old: 100 is before cutoff
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pruned entry, got %d", count)
	}
}

func TestPrivacyFilter(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-privacy-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	defer s.Close()

	filter := privacy.NewFilter([]string{"SECRET", "API_KEY"})
	svc := NewMemoryServiceWithFilter(s, filter)
	defer svc.Close()

	ctx := context.Background()
	sess, _ := svc.Connect(ctx, "privacy-test")

	// Should be blocked
	err := sess.Log(ctx, "agent", "My SECRET key is 123")
	if err == nil {
		t.Errorf("expected error for blocked content, got nil")
	}

	// Should pass
	err = sess.Log(ctx, "agent", "Normal architectural decision")
	if err != nil {
		t.Errorf("expected no error for clean content, got %v", err)
	}

	// Verify only the clean entry was stored
	results, _ := sess.Ask(ctx, "decision", 5)
	if len(results) != 1 {
		t.Errorf("expected 1 searchable entry, got %d", len(results))
	}
}

func TestExportWithArchived(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "agent-mem-export-archived-*")
	defer os.RemoveAll(tempDir)
	s, _ := store.NewStore(tempDir)
	defer s.Close()

	ctx := context.Background()
	sessionID := "test-session"

	s.Append(ctx, api.LogEntry{SessionID: sessionID, Role: "agent-a", Content: "Active", Timestamp: 1000})
	s.Append(ctx, api.LogEntry{SessionID: sessionID, Role: "agent-b", Content: "Archived", Timestamp: 2000})

	// Archive the second entry
	s.ArchiveEntries(ctx, sessionID)

	// Log a new active entry
	s.Append(ctx, api.LogEntry{SessionID: sessionID, Role: "agent-c", Content: "New", Timestamp: 3000})

	var buf bytes.Buffer
	s.Export(ctx, sessionID, &buf)
	output := buf.String()

	if !strings.Contains(output, "Active") {
		t.Errorf("export should include non-archived 'Active' entry")
	}
	if !strings.Contains(output, "[archived]") {
		t.Errorf("export should tag archived entries with '[archived]'")
	}
	if !strings.Contains(output, "New") {
		t.Errorf("export should include new non-archived entry")
	}
}
