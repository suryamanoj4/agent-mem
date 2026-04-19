package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-memory/pkg/store"
)

func TestMemoryService_EndToEnd(t *testing.T) {
	// Setup temporary directory
	tempDir, err := os.MkdirTemp("", "agent-mem-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize Store and Service
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

	// 1. Connect to session
	sess, err := svc.Connect(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// 2. Log an entry
	err = sess.Log(ctx, role, content)
	if err != nil {
		t.Fatalf("failed to log entry: %v", err)
	}

	// 3. Verify SQLite persistence via Search
	results, err := sess.Ask(ctx, "test")
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected search results, got 0")
	}

	// 4. Verify Asynchronous Markdown Mirroring
	// Give the background worker a moment to flush
	mdPath := filepath.Join(tempDir, "sessions", sessionID, "memory_log.md")
	
	// Poll for file existence and content to handle async lag
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(mdPath); err == nil {
			data, _ := os.ReadFile(mdPath)
			if len(data) > 0 {
				found = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !found {
		t.Errorf("markdown log file was not created or is empty at %s", mdPath)
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

	// 1. Acquire lock
	unlock, err := sess.Lock(ctx, path)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// 2. Check status
	locked, owner, _ := sess.GetLockStatus(ctx, path)
	if !locked || owner == "" {
		t.Errorf("expected file to be locked, got locked=%v, owner=%s", locked, owner)
	}

	// 3. Attempt duplicate lock (should fail)
	_, err = sess.Lock(ctx, path)
	if err == nil {
		t.Errorf("expected error when acquiring already locked file, got nil")
	}

	// 4. Release lock
	unlock()

	// 5. Verify released
	locked, _, _ = sess.GetLockStatus(ctx, path)
	if locked {
		t.Errorf("expected file to be unlocked after release")
	}
}
