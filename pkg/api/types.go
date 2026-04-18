package api

import "context"

// LogEntry represents a single memory record appended by an agent.
type LogEntry struct {
	ID        int64
	SessionID string
	Role      string // e.g., "frontend-agent", "backend-agent"
	Content   string
	Timestamp int64
}

// Session represents an active connection to a specific memory context.
type Session interface {
	// Log appends a new entry to the session's memory.
	Log(ctx context.Context, role, content string) error
	
	// Ask searches the session's memory using natural language.
	Ask(ctx context.Context, query string) ([]LogEntry, error)
	
	// Lock attempts to acquire an advisory lock on a specific file path.
	// It returns a function that must be called to release the lock.
	Lock(ctx context.Context, path string) (func() error, error)
	
	// GetLockStatus checks if a file is currently locked.
	GetLockStatus(ctx context.Context, path string) (bool, string, error) // Returns (isLocked, owner, error)
	
	// ID returns the underlying session identifier.
	ID() string
}

// MemoryService is the "Deep Brain" of the broker, handling session lifecycle and storage.
type MemoryService interface {
	// Connect initializes or retrieves a Session object for the given ID.
	Connect(ctx context.Context, sessionID string) (Session, error)
	
	// Close shuts down the service, ensuring all background buffers are flushed.
	Close() error
}

// Store defines the internal interface for persistence and searching.
type Store interface {
	// Append writes a new log entry to the database and schedules it for Markdown syncing.
	Append(ctx context.Context, entry LogEntry) error
	
	// Search performs a full-text search over the logs for a specific session.
	Search(ctx context.Context, sessionID string, query string) ([]LogEntry, error)
	
	// Close flushes any pending writes and closes the underlying database.
	Close() error
}
