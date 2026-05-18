package decision

import (
	"context"
	"io"
	"time"
)

type CallerInfo struct {
	AgentID    string
	AuthorType AuthorType
}

type callerContextKey struct{}

func WithCaller(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, callerContextKey{}, CallerInfo{AgentID: agentID, AuthorType: AuthorTypeAgent})
}

func WithUser(ctx context.Context) context.Context {
	return context.WithValue(ctx, callerContextKey{}, CallerInfo{AuthorType: AuthorTypeUser})
}

func getCaller(ctx context.Context) (CallerInfo, bool) {
	info, ok := ctx.Value(callerContextKey{}).(CallerInfo)
	return info, ok
}

type DecisionSession struct {
	id       string
	store    DecisionStore
	lockMgr  LockManager
	callerFn func(context.Context) (CallerInfo, bool)
}

func NewDecisionSession(id string, store DecisionStore, lockMgr LockManager) *DecisionSession {
	return &DecisionSession{
		id:       id,
		store:    store,
		lockMgr:  lockMgr,
		callerFn: getCaller,
	}
}

func (s *DecisionSession) ID() string {
	return s.id
}

func (s *DecisionSession) Decide(ctx context.Context, decisionType DecisionType, summary string, opts ...DecideOption) error {
	caller, ok := s.callerFn(ctx)
	if !ok {
		panic("caller context not set: use WithCaller or WithUser")
	}

	d := Decision{
		SessionID:    s.id,
		AgentID:      caller.AgentID,
		AuthorType:   caller.AuthorType,
		DecisionType: decisionType,
		Content:      DecisionContent{Summary: summary},
		Timestamp:    time.Now(),
	}

	for _, opt := range opts {
		opt.apply(&d)
	}

	return s.store.Store(ctx, d)
}

func (s *DecisionSession) GetContext(ctx context.Context) ([]Decision, error) {
	caller, ok := s.callerFn(ctx)
	if !ok {
		panic("caller context not set: use WithCaller or WithUser")
	}

	req := SearchRequest{
		SessionID: s.id,
		Limit:     50,
	}

	if caller.AuthorType == AuthorTypeAgent && caller.AgentID != "" {
		req.ExcludeAgentID = caller.AgentID
	}

	return s.store.Search(ctx, req)
}

func (s *DecisionSession) Search(ctx context.Context, query string, filters SearchFilters) ([]Decision, error) {
	req := SearchRequest{
		SessionID:        s.id,
		Query:           query,
		DecisionTypes:   filters.Types,
		IncludeArchived: filters.IncludeArchived,
		Limit:           filters.Limit,
	}
	return s.store.Search(ctx, req)
}

func (s *DecisionSession) Prefer(ctx context.Context, summary string) error {
	caller, ok := s.callerFn(ctx)
	if !ok {
		panic("caller context not set: use WithUser")
	}

	d := Decision{
		SessionID:    s.id,
		AgentID:      caller.AgentID,
		AuthorType:   AuthorTypeUser,
		DecisionType: DecisionTypePreference,
		Content:      DecisionContent{Summary: summary},
		Timestamp:    time.Now(),
	}

	return s.store.Store(ctx, d)
}

func (s *DecisionSession) Lock(ctx context.Context, path, owner string, ttl time.Duration) (func() error, error) {
	if err := s.lockMgr.Acquire(ctx, s.id, path, owner, ttl); err != nil {
		return nil, err
	}
	return func() error {
		return s.lockMgr.Release(ctx, s.id, path)
	}, nil
}

func (s *DecisionSession) GetLockStatus(ctx context.Context, path string) (bool, string, error) {
	return s.lockMgr.Status(ctx, path)
}

func (s *DecisionSession) ReleaseLock(ctx context.Context, path string) error {
	return s.lockMgr.Release(ctx, s.id, path)
}

func (s *DecisionSession) Export(ctx context.Context, w io.Writer) error {
	return s.store.Export(ctx, s.id, w)
}

func (s *DecisionSession) Compact(ctx context.Context) (int64, error) {
	return s.store.Archive(ctx, s.id)
}

type DecideOption interface {
	apply(d *Decision)
}

type decideOption struct {
	fn func(*Decision)
}

func (o *decideOption) apply(d *Decision) {
	o.fn(d)
}

func WithDiff(diff string) DecideOption {
	return &decideOption{fn: func(d *Decision) {
		d.Content.Diff = diff
	}}
}

func WithTags(tags []string) DecideOption {
	return &decideOption{fn: func(d *Decision) {
		d.Content.Tags = tags
	}}
}

type SearchFilters struct {
	Types            []DecisionType
	AgentID          string
	IncludeArchived  bool
	Limit            int
}

func (f SearchFilters) WithTypes(types ...DecisionType) SearchFilters {
	f.Types = types
	return f
}