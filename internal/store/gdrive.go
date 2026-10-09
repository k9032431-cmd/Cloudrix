package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DriveFile tracks the Google Drive copy of a user's subscription.
type DriveFile struct {
	UserID   int64      `json:"user_id"`
	FileID   string     `json:"file_id"`
	Hash     string     `json:"-"`
	SyncedAt *time.Time `json:"synced_at"`
	Error    string     `json:"error"`
}

func (s *Store) GetDriveFile(ctx context.Context, userID int64) (*DriveFile, error) {
	var f DriveFile
	var synced sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT user_id, file_id, hash, synced_at, error FROM user_gdrive WHERE user_id = ?`, userID).
		Scan(&f.UserID, &f.FileID, &f.Hash, &synced, &f.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.SyncedAt = ptrUnix(synced)
	return &f, nil
}

func (s *Store) ListDriveFiles(ctx context.Context) ([]*DriveFile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, file_id, hash, synced_at, error FROM user_gdrive ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DriveFile
	for rows.Next() {
		var f DriveFile
		var synced sql.NullInt64
		if err := rows.Scan(&f.UserID, &f.FileID, &f.Hash, &synced, &f.Error); err != nil {
			return nil, err
		}
		f.SyncedAt = ptrUnix(synced)
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (s *Store) SaveDriveFile(ctx context.Context, f *DriveFile) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_gdrive (user_id, file_id, hash, synced_at, error) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET file_id = excluded.file_id, hash = excluded.hash, synced_at = excluded.synced_at, error = excluded.error`,
		f.UserID, f.FileID, f.Hash, nullUnix(f.SyncedAt), f.Error)
	return err
}

func (s *Store) DeleteDriveFile(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_gdrive WHERE user_id = ?`, userID)
	return err
}
