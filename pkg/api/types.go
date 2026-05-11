package api

import (
	"context"
	"io"
	"time"
)

// LogEntry represents a single memory record appended by an agent.
type LogEntry struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Timestamp int64
	Archived  bool
}

// SessionInfo summarizes a session for listing.
type SessionInfo struct {
	ID          string
	EntryCount  int
	ActiveCount int
	UpdatedAt   int64
}

// Session represents an active connection to a specific memory context.
type Session interface {
	Log(ctx context.Context, role, content string) error
	Ask(ctx context.Context, query string) ([]LogEntry, error)
	Lock(ctx context.Context, path string, owner string, ttl time.Duration) (func() error, error)
	ReleaseLock(ctx context.Context, path string) error
	GetLockStatus(ctx context.Context, path string) (bool, string, error)
	Compact(ctx context.Context) (int64, error)
	Export(ctx context.Context, w io.Writer) error
	ID() string
}

// MemoryService is the "Deep Brain" of the broker.
type MemoryService interface {
	Connect(ctx context.Context, sessionID string) (Session, error)
	Close() error
}

// Store defines the internal interface for persistence.
type Store interface {
	Append(ctx context.Context, entry LogEntry) error
	Search(ctx context.Context, sessionID string, query string) ([]LogEntry, error)
	Export(ctx context.Context, sessionID string, w io.Writer) error
	ArchiveEntries(ctx context.Context, sessionID string) (int64, error)
	ListSessions(ctx context.Context) ([]SessionInfo, error)
	PruneEntries(ctx context.Context, olderThan time.Duration) (int64, error)
	AcquireLock(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error
	ReleaseLock(ctx context.Context, sessionID, path string) error
	GetLockStatus(ctx context.Context, path string) (bool, string, error)
	Close() error
}
