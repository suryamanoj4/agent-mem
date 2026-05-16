package decision

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type SQLiteLockManager struct {
	db *sql.DB
}

func NewSQLiteLockManager(db *sql.DB) *SQLiteLockManager {
	return &SQLiteLockManager{db: db}
}

func (m *SQLiteLockManager) Acquire(ctx context.Context, sessionID, path, owner string, ttl time.Duration) error {
	now := time.Now().Unix()
	if _, err := m.db.ExecContext(ctx, `DELETE FROM locks WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("failed to clean expired locks: %w", err)
	}

	var existingOwner string
	err := m.db.QueryRowContext(ctx,
		`SELECT owner FROM locks WHERE path = ? AND session_id = ? AND expires_at > ?`,
		path, sessionID, now,
	).Scan(&existingOwner)
	if err == nil {
		return fmt.Errorf("file is already locked by %s", existingOwner)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check lock status: %w", err)
	}

	err = m.db.QueryRowContext(ctx,
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
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO locks (path, session_id, owner, acquired_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		path, sessionID, owner, now, expiresAt,
	)
	return err
}

func (m *SQLiteLockManager) Release(ctx context.Context, sessionID, path string) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM locks WHERE path = ? AND session_id = ?`, path, sessionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no lock found for path %s in session %s", path, sessionID)
	}
	return nil
}

func (m *SQLiteLockManager) Status(ctx context.Context, path string) (bool, string, error) {
	now := time.Now().Unix()
	if _, err := m.db.ExecContext(ctx, `DELETE FROM locks WHERE expires_at <= ?`, now); err != nil {
		return false, "", fmt.Errorf("failed to clean expired locks: %w", err)
	}

	var owner string
	err := m.db.QueryRowContext(ctx, `SELECT owner FROM locks WHERE path = ? AND expires_at > ?`, path, now).Scan(&owner)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, owner, nil
}