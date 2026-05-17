//go:build sqlite_fts5

package decision

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteDecisionStore_MigrateFromLogs(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	db, err := sql.Open("sqlite3", filepath.Join(dir, "decisions.db")+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		archived INTEGER NOT NULL DEFAULT 0
	);
	INSERT INTO memory_logs (session_id, role, content, timestamp) VALUES ('s1', 'backend', 'Using Go for auth', 1234567890);
	INSERT INTO memory_logs (session_id, role, content, timestamp) VALUES ('s1', 'frontend', 'React for UI', 1234567891);
	`)
	if err != nil {
		t.Fatalf("failed to create legacy table: %v", err)
	}

	count, err := store.MigrateFromLogs(context.Background())
	if err != nil {
		t.Fatalf("MigrateFromLogs failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 migrated, got %d", count)
	}

	hasLegacy, err := store.HasLegacyData(context.Background())
	if err != nil {
		t.Fatalf("HasLegacyData failed: %v", err)
	}
	if !hasLegacy {
		t.Error("expected HasLegacyData to return true after migration")
	}

	results, err := store.Search(context.Background(), SearchRequest{SessionID: "s1", Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(results))
	}
}

func TestSQLiteDecisionStore_Store(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	d := Decision{
		SessionID:    "s1",
		AgentID:      "agent-1",
		AuthorType:   AuthorTypeAgent,
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "Using PostgreSQL", Tags: []string{"database"}},
	}

	if err := store.Store(ctx, d); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

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

func TestSQLiteDecisionStore_GetByType(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	store.Store(ctx, Decision{SessionID: "s1", AgentID: "a1", DecisionType: DecisionTypeArchitecture, Content: DecisionContent{Summary: "arch"}})
	store.Store(ctx, Decision{SessionID: "s1", AgentID: "a1", DecisionType: DecisionTypePlan, Content: DecisionContent{Summary: "plan"}})

	results, err := store.GetByType(ctx, "s1", DecisionTypeArchitecture)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
}

func TestSQLiteDecisionStore_ListSessions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d1"}})
	store.Store(ctx, Decision{SessionID: "s1", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d2"}})
	store.Store(ctx, Decision{SessionID: "s2", DecisionType: DecisionTypeNote, Content: DecisionContent{Summary: "d3"}})

	sessions, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	sessionsMap := make(map[string]int)
	for _, s := range sessions {
		sessionsMap[s.SessionID] = s.TotalDecisions
	}
	if sessionsMap["s1"] != 2 {
		t.Errorf("s1 expected 2, got %d", sessionsMap["s1"])
	}
	if sessionsMap["s2"] != 1 {
		t.Errorf("s2 expected 1, got %d", sessionsMap["s2"])
	}
}

func TestSQLiteDecisionStore_LockManager(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	db, err := sql.Open("sqlite3", filepath.Join(dir, "decisions.db")+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	lockMgr := NewSQLiteLockManager(db)
	ctx := context.Background()

	if err := lockMgr.Acquire(ctx, "s1", "/path/to/file.go", "owner1", time.Hour); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	locked, owner, err := lockMgr.Status(ctx, "/path/to/file.go")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !locked {
		t.Error("expected locked")
	}
	if owner != "owner1" {
		t.Errorf("owner mismatch: got %q", owner)
	}

	if err := lockMgr.Release(ctx, "s1", "/path/to/file.go"); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	locked, _, err = lockMgr.Status(ctx, "/path/to/file.go")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if locked {
		t.Error("expected unlocked after Release")
	}
}

func TestSQLiteDecisionStore_Export(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteDecisionStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	store.Store(ctx, Decision{
		SessionID:    "s1",
		AgentID:      "agent-1",
		DecisionType: DecisionTypeArchitecture,
		Content:      DecisionContent{Summary: "arch decision", Diff: "--- a/foo\n+++ b/foo"},
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