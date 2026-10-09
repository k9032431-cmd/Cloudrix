package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) AddAudit(ctx context.Context, e *model.AuditEntry) error {
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (admin_id, admin, action, target, details, ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.AdminID, e.Admin, e.Action, e.Target, e.Details, e.IP, unix(e.CreatedAt))
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit, offset int) ([]*model.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, admin_id, admin, action, target, details, ip, created_at FROM audit_log ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		var created int64
		if err := rows.Scan(&e.ID, &e.AdminID, &e.Admin, &e.Action, &e.Target, &e.Details, &e.IP, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = fromUnix(created)
		out = append(out, &e)
	}
	return out, rows.Err()
}
