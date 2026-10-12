package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

const adminCols = `id, username, password_hash, role, disabled, user_limit, created_at,
	expire_at, data_limit, used_traffic, max_user_data, max_user_devices`

func scanAdmin(row interface{ Scan(...any) error }) (*model.Admin, error) {
	var a model.Admin
	var created int64
	var expire sql.NullInt64
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Role, &a.Disabled, &a.UserLimit, &created,
		&expire, &a.DataLimit, &a.UsedTraffic, &a.MaxUserData, &a.MaxUserDevices)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = fromUnix(created)
	a.ExpireAt = ptrUnix(expire)
	return &a, nil
}

func (s *Store) CreateAdmin(ctx context.Context, a *model.Admin) error {
	a.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO admins (username, password_hash, role, disabled, user_limit, created_at,
		expire_at, data_limit, max_user_data, max_user_devices) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Username, a.PasswordHash, a.Role, a.Disabled, a.UserLimit, unix(a.CreatedAt),
		nullUnix(a.ExpireAt), a.DataLimit, a.MaxUserData, a.MaxUserDevices)
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateAdmin(ctx context.Context, a *model.Admin) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admins SET password_hash = ?, role = ?, disabled = ?, user_limit = ?,
		expire_at = ?, data_limit = ?, used_traffic = ?, max_user_data = ?, max_user_devices = ? WHERE id = ?`,
		a.PasswordHash, a.Role, a.Disabled, a.UserLimit,
		nullUnix(a.ExpireAt), a.DataLimit, a.UsedTraffic, a.MaxUserData, a.MaxUserDevices, a.ID)
	return affected(res, err)
}

func (s *Store) DeleteAdmin(ctx context.Context, id int64) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE admin_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM admins WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) GetAdmin(ctx context.Context, id int64) (*model.Admin, error) {
	return scanAdmin(s.db.QueryRowContext(ctx, `SELECT `+adminCols+` FROM admins WHERE id = ?`, id))
}

func (s *Store) GetAdminByUsername(ctx context.Context, username string) (*model.Admin, error) {
	return scanAdmin(s.db.QueryRowContext(ctx, `SELECT `+adminCols+` FROM admins WHERE username = ?`, username))
}

func (s *Store) ListAdmins(ctx context.Context) ([]*model.Admin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+adminCols+` FROM admins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Admin{}
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func affected(res sql.Result, err error) error {
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
