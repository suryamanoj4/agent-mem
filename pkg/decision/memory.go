package decision

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type MemoryDecisionStore struct {
	mu        sync.RWMutex
	decisions []Decision
	nextID    int64
}

func NewMemoryDecisionStore() *MemoryDecisionStore {
	return &MemoryDecisionStore{
		decisions: make([]Decision, 0),
	}
}

func (s *MemoryDecisionStore) Store(ctx context.Context, d Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}
	d.ID = s.nextID
	s.nextID++

	s.decisions = append(s.decisions, d)
	return nil
}

func (s *MemoryDecisionStore) Search(ctx context.Context, req SearchRequest) ([]Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	query := strings.ToLower(req.Query)
	var results []Decision
	for _, d := range s.decisions {
		if d.SessionID != req.SessionID {
			continue
		}
		if d.Archived && !req.IncludeArchived {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Content.Summary), query) {
			continue
		}
		if len(req.DecisionTypes) > 0 && !containsDecisionType(req.DecisionTypes, d.DecisionType) {
			continue
		}
		if req.AgentID != "" && d.AgentID != req.AgentID {
			continue
		}
		results = append(results, d)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func containsDecisionType(types []DecisionType, target DecisionType) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}

func (s *MemoryDecisionStore) GetByType(ctx context.Context, sessionID string, decisionType DecisionType) ([]Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Decision
	for _, d := range s.decisions {
		if d.SessionID == sessionID && d.DecisionType == decisionType && !d.Archived {
			results = append(results, d)
		}
	}
	return results, nil
}

func (s *MemoryDecisionStore) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionMap := make(map[string]SessionSummary)
	for _, d := range s.decisions {
		if d.Archived {
			continue
		}
		sum := sessionMap[d.SessionID]
		sum.SessionID = d.SessionID
		sum.TotalDecisions++
		if d.Timestamp.After(sum.UpdatedAt) {
			sum.UpdatedAt = d.Timestamp
		}
		sessionMap[d.SessionID] = sum
	}

	var results []SessionSummary
	for _, sum := range sessionMap {
		results = append(results, sum)
	}
	return results, nil
}

func (s *MemoryDecisionStore) Archive(ctx context.Context, sessionID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int64
	for i := range s.decisions {
		if s.decisions[i].SessionID == sessionID && !s.decisions[i].Archived {
			s.decisions[i].Archived = true
			count++
		}
	}
	return count, nil
}

func (s *MemoryDecisionStore) Export(ctx context.Context, sessionID string, w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.decisions {
		if d.SessionID != sessionID {
			continue
		}
		ts := d.Timestamp.Format(time.RFC3339)
		fmt.Fprintf(w, "### [%s] %s (%s)\n\n%s\n\n---\n", ts, d.AgentID, d.DecisionType, d.Content.Summary)
	}
	return nil
}

func (s *MemoryDecisionStore) Close() error {
	return nil
}