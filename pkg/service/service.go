package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agent-memory/pkg/api"
)

// memoryService implements api.MemoryService.
type memoryService struct {
	store api.Store
	
	mu       sync.RWMutex
	sessions map[string]*session
	
	// Global lock manager for simplicity in Phase 1
	locksMu sync.Mutex
	locks   map[string]string // path -> ownerID
}

// NewMemoryService creates a new instance of the deep MemoryService.
func NewMemoryService(store api.Store) api.MemoryService {
	return &memoryService{
		store:    store,
		sessions: make(map[string]*session),
		locks:    make(map[string]string),
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

// session implements api.Session.
type session struct {
	id      string
	service *memoryService
}

func (s *session) ID() string {
	return s.id
}

func (s *session) Log(ctx context.Context, role, content string) error {
	// In Phase 5, we will add the privacy interceptor here.
	entry := api.LogEntry{
		SessionID: s.id,
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}
	return s.service.store.Append(ctx, entry)
}

func (s *session) Ask(ctx context.Context, query string) ([]api.LogEntry, error) {
	return s.service.store.Search(ctx, s.id, query)
}

func (s *session) Lock(ctx context.Context, path string) (func() error, error) {
	s.service.locksMu.Lock()
	defer s.service.locksMu.Unlock()

	// Use a dummy owner ID for now; in a real scenario, this might come from the agent context
	ownerID := "agent-context" 

	if existingOwner, locked := s.service.locks[path]; locked {
		return nil, fmt.Errorf("file is already locked by %s", existingOwner)
	}

	s.service.locks[path] = ownerID
	
	unlockFunc := func() error {
		s.service.locksMu.Lock()
		defer s.service.locksMu.Unlock()
		delete(s.service.locks, path)
		return nil
	}

	return unlockFunc, nil
}

func (s *session) GetLockStatus(ctx context.Context, path string) (bool, string, error) {
	s.service.locksMu.Lock()
	defer s.service.locksMu.Unlock()

	owner, locked := s.service.locks[path]
	return locked, owner, nil
}
