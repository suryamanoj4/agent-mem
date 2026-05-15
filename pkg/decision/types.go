package decision

import (
	"time"
)

type DecisionType string

const (
	DecisionTypeArchitecture DecisionType = "architecture"
	DecisionTypeCodeChange  DecisionType = "code_change"
	DecisionTypePlan        DecisionType = "plan"
	DecisionTypePreference   DecisionType = "preference"
	DecisionTypeNote        DecisionType = "note"
)

type AuthorType string

const (
	AuthorTypeAgent AuthorType = "agent"
	AuthorTypeUser  AuthorType = "user"
)

type DecisionContent struct {
	Summary string   `json:"summary"`
	Diff    string   `json:"diff,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type Decision struct {
	ID           int64
	SessionID    string
	AgentID      string
	AuthorType   AuthorType
	DecisionType DecisionType
	Content      DecisionContent
	Timestamp    time.Time
	Archived     bool
}

type SessionSummary struct {
	SessionID       string
	TotalDecisions  int
	ActiveCount    int
	UpdatedAt      time.Time
}