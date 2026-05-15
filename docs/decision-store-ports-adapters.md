# Plan: Ports & Adapters for Structured Decision Store

## Context
Redesign agent-mem's flat `memory_logs` table into a structured decision store with proper attribution, types, and ports & adapters architecture.

## Current State
- Flat schema: `memory_logs(id, session_id, role TEXT, content TEXT, timestamp, archived)`
- Single `SQLStore` implementation with no interface abstraction
- No decision types, no agent_id/author_type attribution
- FTS5 search but no structured query capabilities

## Goal
Ports & adapters pattern with:
1. Clear port interfaces (DecisionStore, LockManager, DecisionExtractor)
2. Production SQLite+FTS5 adapter
3. In-memory test adapter
4. Structured types for decisions

---

## Phase 1: Port Interfaces

### 1.1 Types (`pkg/decision/types.go`)

```go
type DecisionType string  // "architecture"|"code_change"|"plan"|"preference"|"note"
type AuthorType   string  // "agent"|"user"

type DecisionContent struct {
    Summary string   `json:"summary"`
    Diff    string?  `json:"diff,omitempty"`
    Tags    []string `json:"tags,omitempty"`
}

type Decision struct {
    ID           int64
    SessionID    string
    AgentID      string
    AuthorType   AuthorType
    DecisionType DecisionType
    Content      DecisionContent  // JSON
    Timestamp    int64
    Archived     bool
}

type SessionSummary struct {
    SessionID      string
    TotalDecisions int
    ActiveCount    int
    UpdatedAt      int64
}
```

### 1.2 DecisionStore Port (`pkg/decision/store.go`)

```go
type DecisionStore interface {
    // Store a new decision
    Store(ctx context.Context, d Decision) error

    // Search with filters
    Search(ctx context.Context, req SearchRequest) ([]Decision, error)

    // Cross-agent retrieval: exclude caller's decisions by default
    GetForAgent(ctx context.Context, sessionID, callerAgentID string, req SearchRequest) ([]Decision, error)

    // List all sessions with decision counts
    ListSessions(ctx context.Context) ([]SessionSummary, error)

    // Archive a decision
    Archive(ctx context.Context, id int64) error

    // Export session as JSON
    Export(ctx context.Context, sessionID string) ([]Decision, error)

    // Lock management (delegates to LockManager port)
    AcquireLock(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error
    ReleaseLock(ctx context.Context, sessionID, path string) error
    GetLockStatus(ctx context.Context, path string) (locked bool, owner string, err error)

    Close() error
}

type SearchRequest struct {
    SessionID    string
    Query        string  // FTS5 query
    AgentID      string? // filter by agent
    DecisionType string? // filter by type
    AuthorType   string? // filter by author (agent|user)
    IncludeArchived bool
    Limit        int
}
```

### 1.3 LockManager Port (`pkg/decision/locks.go`)

```go
type LockManager interface {
    Acquire(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error
    Release(ctx context.Context, sessionID, path string) error
    Status(ctx context.Context, path string) (locked bool, owner string, err error)
    Refresh(ctx context.Context, sessionID, path string, ttl time.Duration) error
}
```

### 1.4 DecisionExtractor Port (`pkg/decision/extractor.go`)

```go
// Extracts structured decisions from agent context (AI-powered or rule-based)
type DecisionExtractor interface {
    Extract(ctx context.Context, agentID string, rawContent string) ([]DecisionContent, error)
    DetectType(rawContent string) DecisionType
    DetectTags(rawContent string) []string
}
```

---

## Phase 2: Adapters

### 2.1 SQLite DecisionStore Adapter (`pkg/decision/sqlite.go`)

- New schema:
```sql
CREATE TABLE decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    author_type TEXT NOT NULL CHECK(author_type IN ('agent','user')),
    decision_type TEXT NOT NULL CHECK(decision_type IN ('architecture','code_change','plan','preference','note')),
    content TEXT NOT NULL,  -- JSON
    timestamp INTEGER NOT NULL,
    archived INTEGER DEFAULT 0
);

CREATE INDEX idx_decisions_session ON decisions(session_id, archived);
CREATE INDEX idx_decisions_agent ON decisions(session_id, agent_id);
CREATE INDEX idx_decisions_type ON decisions(session_id, decision_type);

CREATE VIRTUAL TABLE decisions_fts USING fts5(
    content,
    content='decisions',
    content_rowid='id'
);

CREATE TRIGGER decisions_ai AFTER INSERT ON decisions BEGIN
    INSERT INTO decisions_fts(rowid, content) VALUES (new.id, new.content);
END;
```

- FTS5 search across `content` column (stores JSON, FTS5 tokenizes it)
- Cross-agent query: `WHERE agent_id != ? OR author_type = 'user'`

### 2.2 In-Memory Test Adapter (`pkg/decision/memory.go`)

```go
type MemoryDecisionStore struct {
    mu    sync.RWMutex
    items []Decision
    locks map[string]lockEntry
}

func NewMemoryDecisionStore() *MemoryDecisionStore {
    return &MemoryDecisionStore{
        items: make([]Decision, 0),
        locks: make(map[string]lockEntry),
    }
}
```

- Thread-safe with `sync.RWMutex`
- In-memory FTS5 simulation via regex or string matching
- Suitable for integration tests without file system

### 2.3 File LockManager Adapter (`pkg/decision/file_lock.go`)

- Wraps SQLite locks table for cross-process coordination
- Could also use `fcntl`/`flock` for pure file-level locking
- Provides TTL-based lock expiration

---

## Phase 3: Schema Migration

### Migration Strategy
1. Create new `decisions` table alongside `memory_logs`
2. Backfill script to convert existing entries
3. V2 API uses new table, old table preserved for rollback
4. Deprecate `memory_logs` in future version

### Backfill Logic
```go
func MigrateEntry(entry api.LogEntry) Decision {
    return Decision{
        SessionID:    entry.SessionID,
        AgentID:      entry.Role,  // old Role becomes AgentID
        AuthorType:   "agent",
        DecisionType: DetectType(entry.Content),
        Content:      DecisionContent{Summary: entry.Content},
        Timestamp:    entry.Timestamp,
        Archived:     entry.Archived,
    }
}
```

---

## Phase 4: API Changes

### Updated Session Interface
```go
type Session interface {
    // Store a decision
    Decide(ctx context.Context, agentID string, authorType AuthorType, decisionType DecisionType, content DecisionContent) error

    // Search decisions
    Search(ctx context.Context, req SearchRequest) ([]Decision, error)

    // Cross-agent retrieval (excludes caller's own decisions)
    GetOtherDecisions(ctx context.Context, callerAgentID string, req SearchRequest) ([]Decision, error)

    // Lock management
    Lock(ctx context.Context, path string, owner string, ttl time.Duration) (func() error, error)
    ReleaseLock(ctx context.Context, path string) error
    GetLockStatus(ctx context.Context, path string) (bool, string, error)

    // Session management
    ID() string
    Export(ctx context.Context) ([]Decision, error)
    Compact(ctx context.Context) (int64, error)
}
```

---

## Phase 5: File Structure

```
pkg/decision/
    types.go           # Domain types
    ports.go           # Port interfaces (DecisionStore, LockManager, DecisionExtractor)
    sqlite.go          # SQLite production adapter
    memory.go          # In-memory test adapter
    file_lock.go       # File-based lock adapter
    extractor.go       # Default rule-based extractor
```

---

## Trade-offs

### Ports & Adapters Benefits
1. **Testability**: Swap SQLite for in-memory adapter in tests
2. **Flexibility**: Different adapters for different environments (file-based, cloud storage)
3. **Clean boundaries**: Domain logic independent of storage details
4. **Cross-agent awareness**: Structured attribution enables filtering by agent

### Ports & Adapters Costs
1. **Indirection overhead**: Extra interface layer adds complexity
2. **Schema coupling**: Ports may need to evolve if domain model changes
3. **Testing complexity**: In-memory adapter must faithfully simulate SQLite behavior

### Alternative Considered: Repository Pattern
- More aligned with DDD
- Single `DecisionRepository` instead of multiple ports
- Decision: Ports is better for this case because we have distinct capabilities (store, lock, extract) that don't naturally compose into one interface

---

## Implementation Order

1. [ ] Define `types.go` with all domain types
2. [ ] Define `ports.go` with interfaces
3. [ ] Implement `MemoryDecisionStore` (simplest, for tests)
4. [ ] Implement `SQLiteDecisionStore` (production)
5. [ ] Add `DecisionExtractor` default implementation
6. [ ] Add `FileLockManager` adapter
7. [ ] Write integration tests using in-memory adapter
8. [ ] Migration script for existing data
9. [ ] Update service layer to use new interfaces