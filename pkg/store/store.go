package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agent-memory/pkg/api"
	_ "github.com/mattn/go-sqlite3"
)

// SQLStore implements the api.Store interface using SQLite and a background Markdown syncer.
type SQLStore struct {
	db           *sql.DB
	baseDir      string
	syncChan     chan api.LogEntry
	wg           sync.WaitGroup
	ctx          context.Context
	cancelWorker context.CancelFunc
}

// NewStore initializes a new SQLStore, creating the database and starting the sync worker.
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

	ctx, cancel := context.WithCancel(context.Background())
	s := &SQLStore{
		db:           db,
		baseDir:      baseDir,
		syncChan:     make(chan api.LogEntry, 100), // Buffer to prevent blocking the main thread
		ctx:          ctx,
		cancelWorker: cancel,
	}

	s.wg.Add(1)
	go s.syncWorker()

	return s, nil
}

func initSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS memory_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp INTEGER NOT NULL
	);
	
	-- FTS5 table for fast search
	CREATE VIRTUAL TABLE IF NOT EXISTS memory_logs_fts USING fts5(
		content,
		content='memory_logs',
		content_rowid='id'
	);
	
	-- Triggers to keep FTS table in sync
	CREATE TRIGGER IF NOT EXISTS memory_logs_ai AFTER INSERT ON memory_logs BEGIN
		INSERT INTO memory_logs_fts(rowid, content) VALUES (new.id, new.content);
	END;
	`
	_, err := db.Exec(query)
	return err
}

func (s *SQLStore) Append(ctx context.Context, entry api.LogEntry) error {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}

	query := `INSERT INTO memory_logs (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, query, entry.SessionID, entry.Role, entry.Content, entry.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	entry.ID = id

	// Send to background worker for Markdown mirroring (non-blocking if channel is not full)
	select {
	case s.syncChan <- entry:
		// Sent successfully
	default:
		// If the channel is full, we log a warning but don't fail the append.
		// In a production system, we might want to scale up workers or block if consistency is critical.
		log.Printf("Warning: sync channel full, markdown sync delayed for entry %d\n", entry.ID)
		go func() { s.syncChan <- entry }() // Send asynchronously to eventually catch up
	}

	return nil
}

func (s *SQLStore) Search(ctx context.Context, sessionID string, query string) ([]api.LogEntry, error) {
	// Search using FTS5 MATCH, joining back to the original table to get role and timestamp
	sqlQuery := `
	SELECT m.id, m.session_id, m.role, m.content, m.timestamp 
	FROM memory_logs m
	JOIN memory_logs_fts f ON m.id = f.rowid
	WHERE m.session_id = ? AND memory_logs_fts MATCH ?
	ORDER BY f.rank
	LIMIT 10
	`

	rows, err := s.db.QueryContext(ctx, sqlQuery, sessionID, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()

	var results []api.LogEntry
	for rows.Next() {
		var e api.LogEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Role, &e.Content, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, e)
	}
	return results, nil
}

func (s *SQLStore) syncWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			// Flush remaining entries before exiting
			s.flushRemaining()
			return
		case entry := <-s.syncChan:
			s.writeMarkdown(entry)
		}
	}
}

func (s *SQLStore) flushRemaining() {
	for {
		select {
		case entry := <-s.syncChan:
			s.writeMarkdown(entry)
		default:
			return // Channel is empty
		}
	}
}

func (s *SQLStore) writeMarkdown(entry api.LogEntry) {
	sessionDir := filepath.Join(s.baseDir, "sessions", entry.SessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		log.Printf("Error creating session directory %s: %v\n", sessionDir, err)
		return
	}

	// We append all logs for a session to a single file, or we could split by date.
	// For now, let's use a single chronological log file per session.
	filePath := filepath.Join(sessionDir, "memory_log.md")
	
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening markdown file %s: %v\n", filePath, err)
		return
	}
	defer file.Close()

	timestamp := time.Unix(entry.Timestamp, 0).Format(time.RFC3339)
	content := fmt.Sprintf("### [%s] %s\n\n%s\n\n---\n", timestamp, entry.Role, entry.Content)

	if _, err := file.WriteString(content); err != nil {
		log.Printf("Error writing to markdown file %s: %v\n", filePath, err)
	}
}

func (s *SQLStore) Close() error {
	s.cancelWorker()
	s.wg.Wait() // Wait for worker to finish flushing and exit
	close(s.syncChan)
	return s.db.Close()
}
