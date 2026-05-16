package decision

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *SQLiteDecisionStore) MigrateFromLogs(ctx context.Context) (int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, timestamp, archived FROM memory_logs ORDER BY id`,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to query memory_logs: %w", err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var id int64
		var sessionID, role, content string
		var timestamp int64
		var archived int

		if err := rows.Scan(&id, &sessionID, &role, &content, &timestamp, &archived); err != nil {
			return 0, fmt.Errorf("failed to scan memory_logs row: %w", err)
		}

		decisionContent := DecisionContent{Summary: content}
		contentJSON, err := json.Marshal(decisionContent)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal content: %w", err)
		}

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO decisions (session_id, agent_id, author_type, content, decision_type, timestamp, archived) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			sessionID,
			role,
			"agent",
			string(contentJSON),
			string(DecisionTypeNote),
			timestamp,
			archived,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to insert decision: %w", err)
		}
		count++
	}

	return count, rows.Err()
}

func (s *SQLiteDecisionStore) HasLegacyData(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_logs`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *SQLiteDecisionStore) InitLegacySchema(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS memory_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		archived INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_memory_logs_session ON memory_logs(session_id, archived);

	CREATE VIRTUAL TABLE IF NOT EXISTS memory_logs_fts USING fts5(
		content,
		content='memory_logs',
		content_rowid='id'
	);

	CREATE TRIGGER IF NOT EXISTS memory_logs_ai AFTER INSERT ON memory_logs BEGIN
		INSERT INTO memory_logs_fts(rowid, content) VALUES (new.id, new.content);
	END;
	`
	_, err := s.db.Exec(query)
	return err
}