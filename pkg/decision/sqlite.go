package decision

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteDecisionStore struct {
	db      *sql.DB
	baseDir string
}

func NewSQLiteDecisionStore(baseDir string) (*SQLiteDecisionStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base dir: %w", err)
	}

	dbPath := filepath.Join(baseDir, "decisions.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initDecisionSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteDecisionStore{db: db, baseDir: baseDir}, nil
}

func initDecisionSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS decisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		author_type TEXT NOT NULL,
		content TEXT NOT NULL,
		decision_type TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		archived INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_decisions_session ON decisions(session_id, archived);
	CREATE INDEX IF NOT EXISTS idx_decisions_agent ON decisions(session_id, agent_id);
	CREATE INDEX IF NOT EXISTS idx_decisions_type ON decisions(decision_type);

	CREATE VIRTUAL TABLE IF NOT EXISTS decisions_fts USING fts5(
		content,
		content='decisions',
		content_rowid='id'
	);

	CREATE TRIGGER IF NOT EXISTS decisions_ai AFTER INSERT ON decisions BEGIN
		INSERT INTO decisions_fts(rowid, content) VALUES (new.id, new.content);
	END;

	CREATE TABLE IF NOT EXISTS locks (
		path TEXT NOT NULL,
		session_id TEXT NOT NULL,
		owner TEXT NOT NULL,
		acquired_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		PRIMARY KEY (path, session_id)
	);

	CREATE INDEX IF NOT EXISTS idx_locks_expires ON locks(expires_at);
	`
	_, err := db.Exec(query)
	return err
}

func (s *SQLiteDecisionStore) Store(ctx context.Context, d Decision) error {
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}

	content, err := json.Marshal(d.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO decisions (session_id, agent_id, author_type, content, decision_type, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
		d.SessionID, d.AgentID, string(d.AuthorType), string(content), string(d.DecisionType), d.Timestamp.Unix(),
	)
	return err
}

func (s *SQLiteDecisionStore) Search(ctx context.Context, req SearchRequest) ([]Decision, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	query := `
	SELECT d.id, d.session_id, d.agent_id, d.author_type, d.content, d.decision_type, d.timestamp, d.archived
	FROM decisions d
	JOIN decisions_fts f ON d.id = f.rowid
	WHERE d.session_id = ? AND d.archived = 0 AND f.content MATCH ?
	ORDER BY d.timestamp DESC
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, req.SessionID, req.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search decisions: %w", err)
	}
	defer rows.Close()

	return scanDecisions(rows)
}

func (s *SQLiteDecisionStore) GetByType(ctx context.Context, sessionID string, decisionType DecisionType) ([]Decision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, agent_id, author_type, content, decision_type, timestamp, archived
		 FROM decisions WHERE session_id = ? AND decision_type = ? AND archived = 0 ORDER BY timestamp DESC`,
		sessionID, string(decisionType),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanDecisions(rows)
}

func (s *SQLiteDecisionStore) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, COUNT(*) as count, MAX(timestamp) as updated_at
		FROM decisions WHERE archived = 0 GROUP BY session_id ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SessionSummary
	for rows.Next() {
		var s SessionSummary
		var updatedAt sql.NullInt64
		if err := rows.Scan(&s.SessionID, &s.TotalDecisions, &updatedAt); err != nil {
			return nil, err
		}
		if updatedAt.Valid {
			s.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (s *SQLiteDecisionStore) Archive(ctx context.Context, sessionID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE decisions SET archived = 1 WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteDecisionStore) PruneEntries(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM decisions WHERE archived = 1 AND timestamp < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteDecisionStore) Export(ctx context.Context, sessionID string, w io.Writer) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, author_type, content, decision_type, timestamp FROM decisions WHERE session_id = ? ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var agentID, authorType, content, decisionType string
		var timestamp int64
		if err := rows.Scan(&agentID, &authorType, &content, &decisionType, &timestamp); err != nil {
			return err
		}
		ts := time.Unix(timestamp, 0).Format(time.RFC3339)
		fmt.Fprintf(w, "### [%s] %s (%s)\n\n%s\n\n---\n", ts, agentID, decisionType, content)
	}
	return rows.Err()
}

func (s *SQLiteDecisionStore) Close() error {
	return s.db.Close()
}

func scanDecisions(rows *sql.Rows) ([]Decision, error) {
	var decisions []Decision
	for rows.Next() {
		var d Decision
		var authorType, content, decisionType string
		var timestamp int64
		var archived int
		if err := rows.Scan(&d.ID, &d.SessionID, &d.AgentID, &authorType, &content, &decisionType, &timestamp, &archived); err != nil {
			return nil, err
		}
		d.AuthorType = AuthorType(authorType)
		d.DecisionType = DecisionType(decisionType)
		d.Timestamp = time.Unix(timestamp, 0)
		d.Archived = archived == 1
		json.Unmarshal([]byte(content), &d.Content)
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}