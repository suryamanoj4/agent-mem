package decision

import (
	"context"
	"io"
)

type SearchRequest struct {
	SessionID     string
	Query         string
	DecisionTypes []DecisionType
	AgentID       string
	AuthorType    AuthorType
	IncludeArchived bool
	Limit         int
}

type DecisionStore interface {
	Store(ctx context.Context, d Decision) error
	Search(ctx context.Context, req SearchRequest) ([]Decision, error)
	GetByType(ctx context.Context, sessionID string, decisionType DecisionType) ([]Decision, error)
	ListSessions(ctx context.Context) ([]SessionSummary, error)
	Archive(ctx context.Context, sessionID string) (int64, error)
	Export(ctx context.Context, sessionID string, w io.Writer) error
	Close() error
}

type LockManager interface {
	Acquire(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error
	Release(ctx context.Context, sessionID, path string) error
	Status(ctx context.Context, path string) (locked bool, owner string, err error)
}

type AgentContext struct {
	SessionID    string
	AgentID      string
	LastOutput   string
	FilesChanged []string
}

type DecisionExtractor interface {
	Extract(ctx context.Context, agentCtx AgentContext) ([]Decision, error)
}