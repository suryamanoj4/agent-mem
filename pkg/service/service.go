package service

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"agent-memory/pkg/api"
	"agent-memory/pkg/privacy"
)

type memoryService struct {
	store          api.Store
	privacyFilter  *privacy.Filter

	mu       sync.RWMutex
	sessions map[string]*session
}

func NewMemoryService(store api.Store) api.MemoryService {
	return &memoryService{
		store:    store,
		sessions: make(map[string]*session),
	}
}

func NewMemoryServiceWithFilter(store api.Store, filter *privacy.Filter) api.MemoryService {
	return &memoryService{
		store:         store,
		privacyFilter: filter,
		sessions:      make(map[string]*session),
	}
}

func (s *memoryService) Connect(ctx context.Context, sessionID string) (api.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[sessionID]; ok {
		return sess, nil
	}

	sess := &session{
		id:      sessionID,
		service: s,
	}
	s.sessions[sessionID] = sess
	return sess, nil
}

func (s *memoryService) Close() error {
	return s.store.Close()
}

type session struct {
	id      string
	service *memoryService
}

func (s *session) ID() string {
	return s.id
}

func (s *session) Log(ctx context.Context, role, content string) error {
	if s.service.privacyFilter != nil {
		if blocked, pattern := s.service.privacyFilter.IsBlocked(content); blocked {
			return fmt.Errorf("content blocked by .mcpignore rule: %s", pattern)
		}
	}
	entry := api.LogEntry{
		SessionID: s.id,
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}
	return s.service.store.Append(ctx, entry)
}

func (s *session) Ask(ctx context.Context, query string, contextSize int) ([]api.LogEntry, error) {
	return s.service.store.Search(ctx, s.id, query, contextSize)
}

func (s *session) Lock(ctx context.Context, path string, owner string, ttl time.Duration) (func() error, error) {
	if err := s.service.store.AcquireLock(ctx, s.id, path, owner, ttl); err != nil {
		return nil, err
	}
	return func() error {
		return s.service.store.ReleaseLock(ctx, s.id, path)
	}, nil
}

func (s *session) ReleaseLock(ctx context.Context, path string) error {
	return s.service.store.ReleaseLock(ctx, s.id, path)
}

func (s *session) GetLockStatus(ctx context.Context, path string) (bool, string, error) {
	return s.service.store.GetLockStatus(ctx, path)
}

func (s *session) Compact(ctx context.Context) (int64, error) {
	return s.service.store.ArchiveEntries(ctx, s.id)
}

func (s *session) Export(ctx context.Context, w io.Writer) error {
	return s.service.store.Export(ctx, s.id, w)
}
