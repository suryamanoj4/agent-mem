package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-memory/pkg/api"
	_ "github.com/mattn/go-sqlite3"
)

type SQLStore struct {
	db      *sql.DB
	baseDir string
}

func NewStore(baseDir string) (*SQLStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base dir: %w", err)
	}

	dbPath := filepath.Join(baseDir, "memory.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLStore{
		db:      db,
		baseDir: baseDir,
	}, nil
}

func initSchema(db *sql.DB) error {
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

func (s *SQLStore) Append(ctx context.Context, entry api.LogEntry) error {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_logs (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		entry.SessionID, entry.Role, entry.Content, entry.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	entry.ID = id
	return nil
}

func (s *SQLStore) Search(ctx context.Context, sessionID string, query string, contextSize int) ([]api.LogEntry, error) {
	// Step 1: Get matching entry positions via FTS5
	if contextSize < 0 {
		contextSize = 5
	}

	matchQuery := `
	WITH ranked AS (
		SELECT id, ROW_NUMBER() OVER (ORDER BY timestamp ASC, id ASC) as rn
		FROM memory_logs
		WHERE session_id = ? AND archived = 0
	)
	SELECT r.id, r.rn
	FROM ranked r
	JOIN memory_logs_fts f ON r.id = f.rowid
	WHERE f.content MATCH ?
	ORDER BY f.rank
	LIMIT 10
	`

	rows, err := s.db.QueryContext(ctx, matchQuery, sessionID, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()

	type match struct {
		id int64
		rn int64
	}
	var matches []match
	for rows.Next() {
		var m match
		if err := rows.Scan(&m.id, &m.rn); err != nil {
			return nil, fmt.Errorf("failed to scan match: %w", err)
		}
		matches = append(matches, m)
	}
	if len(matches) == 0 {
		return nil, nil
	}

	if contextSize == 0 {
		ids := make([]interface{}, len(matches))
		for i, m := range matches {
			ids[i] = m.id
		}
		placeholders := strings.Repeat(",?", len(matches))[1:]
		q := fmt.Sprintf(
			`SELECT id, session_id, role, content, timestamp, archived FROM memory_logs WHERE session_id = ? AND id IN (%s) ORDER BY timestamp ASC, id ASC`,
			placeholders,
		)
		args := []interface{}{sessionID}
		args = append(args, ids...)
		rrows, err := s.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch exact matches: %w", err)
		}
		defer rrows.Close()
		var results []api.LogEntry
		for rrows.Next() {
			var e api.LogEntry
			var archived int
			if err := rrows.Scan(&e.ID, &e.SessionID, &e.Role, &e.Content, &e.Timestamp, &archived); err != nil {
				return nil, fmt.Errorf("failed to scan result: %w", err)
			}
			e.Archived = archived == 1
			results = append(results, e)
		}
		return results, nil
	}

	// Step 2: Compute global window across all matches
	minRN := int64(1<<63 - 1)
	maxRN := int64(0)
	for _, m := range matches {
		lo := m.rn - int64(contextSize)
		if lo < 1 {
			lo = 1
		}
		hi := m.rn + int64(contextSize)
		if hi > maxRN {
			maxRN = hi
		}
		if lo < minRN {
			minRN = lo
		}
	}

	// Step 3: Fetch all entries in the window
	windowQuery := `
	WITH ranked AS (
		SELECT id, session_id, role, content, timestamp, archived,
			   ROW_NUMBER() OVER (ORDER BY timestamp ASC, id ASC) as rn
		FROM memory_logs
		WHERE session_id = ? AND archived = 0
	)
	SELECT id, session_id, role, content, timestamp, archived
	FROM ranked
	WHERE rn BETWEEN ? AND ?
	ORDER BY rn ASC
	`

	wrows, err := s.db.QueryContext(ctx, windowQuery, sessionID, minRN, maxRN)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch window: %w", err)
	}
	defer wrows.Close()

	var results []api.LogEntry
	for wrows.Next() {
		var e api.LogEntry
		var archived int
		if err := wrows.Scan(&e.ID, &e.SessionID, &e.Role, &e.Content, &e.Timestamp, &archived); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		e.Archived = archived == 1
		results = append(results, e)
	}
	return results, nil
}

func (s *SQLStore) ArchiveEntries(ctx context.Context, sessionID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memory_logs SET archived = 1 WHERE session_id = ? AND archived = 0`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to archive entries: %w", err)
	}
	return res.RowsAffected()
}

func (s *SQLStore) ListSessions(ctx context.Context) ([]api.SessionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			session_id,
			COUNT(*) as total,
			SUM(CASE WHEN archived = 0 THEN 1 ELSE 0 END) as active,
			MAX(timestamp) as updated_at
		FROM memory_logs
		GROUP BY session_id
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var results []api.SessionInfo
	for rows.Next() {
		var info api.SessionInfo
		var updatedAt sql.NullInt64
		if err := rows.Scan(&info.ID, &info.EntryCount, &info.ActiveCount, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan session info: %w", err)
		}
		if updatedAt.Valid {
			info.UpdatedAt = updatedAt.Int64
		}
		results = append(results, info)
	}
	return results, nil
}

func (s *SQLStore) PruneEntries(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM memory_logs WHERE archived = 1 AND timestamp < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to prune entries: %w", err)
	}
	return res.RowsAffected()
}

func (s *SQLStore) Export(ctx context.Context, sessionID string, w io.Writer) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content, timestamp, archived FROM memory_logs WHERE session_id = ? ORDER BY timestamp ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to query session logs: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var role, content string
		var timestamp int64
		var archived int
		if err := rows.Scan(&role, &content, &timestamp, &archived); err != nil {
			return fmt.Errorf("failed to scan log entry: %w", err)
		}
		ts := time.Unix(timestamp, 0).Format(time.RFC3339)
		tag := ""
		if archived == 1 {
			tag = " [archived]"
		}
		line := fmt.Sprintf("### [%s]%s %s\n\n%s\n\n---\n", ts, tag, role, content)
		if _, err := io.WriteString(w, line); err != nil {
			return fmt.Errorf("failed to write export: %w", err)
		}
		count++
	}
	if count == 0 {
		io.WriteString(w, "No entries found for this session.\n")
	}
	return rows.Err()
}

func (s *SQLStore) AcquireLock(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM locks WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("failed to clean expired locks: %w", err)
	}

	var existingOwner string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner FROM locks WHERE path = ? AND session_id = ? AND expires_at > ?`,
		path, sessionID, now,
	).Scan(&existingOwner)
	if err == nil {
		return fmt.Errorf("file is already locked by %s", existingOwner)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check lock status: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT owner FROM locks WHERE path = ? AND session_id != ? AND expires_at > ?`,
		path, sessionID, now,
	).Scan(&existingOwner)
	if err == nil {
		return fmt.Errorf("file is locked by %s in a different session", existingOwner)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check cross-session lock: %w", err)
	}

	expiresAt := now + int64(ttl.Seconds())
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO locks (path, session_id, owner, acquired_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		path, sessionID, owner, now, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	return nil
}

func (s *SQLStore) ReleaseLock(ctx context.Context, sessionID, path string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM locks WHERE path = ? AND session_id = ?`,
		path, sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no lock found for path %s in this session", path)
	}
	return nil
}

func (s *SQLStore) GetLockStatus(ctx context.Context, path string) (bool, string, error) {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM locks WHERE expires_at <= ?`, now); err != nil {
		return false, "", fmt.Errorf("failed to clean expired locks: %w", err)
	}
	var owner string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner FROM locks WHERE path = ? AND expires_at > ?`, path, now,
	).Scan(&owner)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check lock status: %w", err)
	}
	return true, owner, nil
}

func (s *SQLStore) Close() error {
	return s.db.Close()
}
