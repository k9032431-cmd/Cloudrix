package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

const userCols = `id, username, sub_token, status, data_limit, used_traffic, lifetime_used, expire_at,
	on_hold_duration, reset_strategy, last_reset_at, device_limit, inbounds, note, admin_id, credentials,
	created_at, online_at, sub_updated_at, sub_user_agent`

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	var expire, online, subUpdated sql.NullInt64
	var lastReset, created int64
	var inbounds, creds string
	err := row.Scan(&u.ID, &u.Username, &u.SubToken, &u.Status, &u.DataLimit, &u.UsedTraffic, &u.LifetimeUsed, &expire,
		&u.OnHoldDuration, &u.ResetStrategy, &lastReset, &u.DeviceLimit, &inbounds, &u.Note, &u.AdminID, &creds,
		&created, &online, &subUpdated, &u.SubUserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.ExpireAt = ptrUnix(expire)
	u.OnlineAt = ptrUnix(online)
	u.SubUpdatedAt = ptrUnix(subUpdated)
	u.LastResetAt = fromUnix(lastReset)
	u.CreatedAt = fromUnix(created)
	if err := json.Unmarshal([]byte(inbounds), &u.Inbounds); err != nil {
		return nil, err
	}
	if u.Inbounds == nil {
		u.Inbounds = []string{}
	}
	if err := json.Unmarshal([]byte(creds), &u.Credentials); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *model.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.LastResetAt = now
	if u.Inbounds == nil {
		u.Inbounds = []string{}
	}
	inbounds, _ := json.Marshal(u.Inbounds)
	creds, _ := json.Marshal(u.Credentials)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, sub_token, status, data_limit, used_traffic, lifetime_used,
		expire_at, on_hold_duration, reset_strategy, last_reset_at, device_limit, inbounds, note, admin_id, credentials, created_at)
		VALUES (?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.SubToken, u.Status, u.DataLimit, nullUnix(u.ExpireAt), u.OnHoldDuration, u.ResetStrategy,
		unix(now), u.DeviceLimit, string(inbounds), u.Note, u.AdminID, string(creds), unix(now))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

// UpdateUser writes every mutable field of u.
func (s *Store) UpdateUser(ctx context.Context, u *model.User) error {
	if u.Inbounds == nil {
		u.Inbounds = []string{}
	}
	inbounds, _ := json.Marshal(u.Inbounds)
	creds, _ := json.Marshal(u.Credentials)
	res, err := s.db.ExecContext(ctx, `UPDATE users SET username = ?, sub_token = ?, status = ?, data_limit = ?, used_traffic = ?,
		expire_at = ?, on_hold_duration = ?, reset_strategy = ?, last_reset_at = ?, device_limit = ?, inbounds = ?, note = ?,
		admin_id = ?, credentials = ? WHERE id = ?`,
		u.Username, u.SubToken, u.Status, u.DataLimit, u.UsedTraffic, nullUnix(u.ExpireAt), u.OnHoldDuration, u.ResetStrategy,
		unix(u.LastResetAt), u.DeviceLimit, string(inbounds), u.Note, u.AdminID, string(creds), u.ID)
	return affected(res, err)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

func (s *Store) GetUserBySubToken(ctx context.Context, token string) (*model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE sub_token = ?`, token))
}

type UserFilter struct {
	AdminID *int64
	Search  string
	Status  model.UserStatus
	Sort    string // field name, prefix "-" for descending
	Limit   int
	Offset  int
}

var userSortable = map[string]string{
	"id": "id", "username": "username", "used_traffic": "used_traffic", "data_limit": "data_limit",
	"expire_at": "expire_at", "created_at": "created_at", "online_at": "online_at",
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]*model.User, int, error) {
	var where []string
	var args []any
	if f.AdminID != nil {
		where = append(where, "admin_id = ?")
		args = append(args, *f.AdminID)
	}
	if f.Search != "" {
		where = append(where, "(username LIKE ? ESCAPE '\\' OR note LIKE ? ESCAPE '\\')")
		pattern := "%" + escapeLike(f.Search) + "%"
		args = append(args, pattern, pattern)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "id DESC"
	if f.Sort != "" {
		field, dir := strings.TrimPrefix(f.Sort, "-"), "ASC"
		if strings.HasPrefix(f.Sort, "-") {
			dir = "DESC"
		}
		if col, ok := userSortable[field]; ok {
			order = col + " " + dir + ", id DESC"
		}
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT ` + userCols + ` FROM users` + cond + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// AllUsers returns every user; used by background jobs and core config generation.
func (s *Store) AllUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CountUsersByAdmin(ctx context.Context, adminID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE admin_id = ?`, adminID).Scan(&n)
	return n, err
}

func (s *Store) MarkSubscriptionFetched(ctx context.Context, id int64, userAgent string) error {
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET sub_updated_at = ?, sub_user_agent = ? WHERE id = ?`,
		unix(time.Now()), userAgent, id)
	return err
}

func (s *Store) ClearOnline(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET online_at = NULL WHERE id = ?`, id)
	return err
}

// AddTraffic records usage reported by a core for one user.
func (s *Store) AddTraffic(ctx context.Context, userID, bytes int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET used_traffic = used_traffic + ?, lifetime_used = lifetime_used + ?, online_at = ? WHERE id = ?`,
		bytes, bytes, unix(now), userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO traffic_daily (day, user_id, bytes) VALUES (?, ?, ?)
		ON CONFLICT(day, user_id) DO UPDATE SET bytes = bytes + excluded.bytes`,
		now.Format("2006-01-02"), userID, bytes); err != nil {
		return err
	}
	return tx.Commit()
}

type UserStats struct {
	Total    int   `json:"total"`
	Active   int   `json:"active"`
	Disabled int   `json:"disabled"`
	Limited  int   `json:"limited"`
	Expired  int   `json:"expired"`
	OnHold   int   `json:"on_hold"`
	Online   int   `json:"online"`
	Traffic  int64 `json:"traffic"`
	// ExpiringSoon counts active users whose subscription ends within 7 days.
	ExpiringSoon int `json:"expiring_soon"`
}

func (s *Store) UserStats(ctx context.Context, adminID *int64, onlineSince time.Time) (UserStats, error) {
	var st UserStats
	cond, args := "", []any{}
	if adminID != nil {
		cond, args = " WHERE admin_id = ?", append(args, *adminID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*), COALESCE(SUM(lifetime_used), 0) FROM users`+cond+` GROUP BY status`, args...)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var status model.UserStatus
		var n int
		var traffic int64
		if err := rows.Scan(&status, &n, &traffic); err != nil {
			return st, err
		}
		st.Total += n
		st.Traffic += traffic
		switch status {
		case model.StatusActive:
			st.Active = n
		case model.StatusDisabled:
			st.Disabled = n
		case model.StatusLimited:
			st.Limited = n
		case model.StatusExpired:
			st.Expired = n
		case model.StatusOnHold:
			st.OnHold = n
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	onlineCond := " WHERE online_at >= ?"
	onlineArgs := []any{unix(onlineSince)}
	if adminID != nil {
		onlineCond += " AND admin_id = ?"
		onlineArgs = append(onlineArgs, *adminID)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+onlineCond, onlineArgs...).Scan(&st.Online); err != nil {
		return st, err
	}
	now := time.Now()
	expCond := " WHERE status = ? AND expire_at IS NOT NULL AND expire_at >= ? AND expire_at < ?"
	expArgs := []any{model.StatusActive, unix(now), unix(now.Add(7 * 24 * time.Hour))}
	if adminID != nil {
		expCond += " AND admin_id = ?"
		expArgs = append(expArgs, *adminID)
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+expCond, expArgs...).Scan(&st.ExpiringSoon)
	return st, err
}

type DailyTraffic struct {
	Day   string `json:"day"`
	Bytes int64  `json:"bytes"`
}

func (s *Store) TrafficHistory(ctx context.Context, userID *int64, adminID *int64, days int) ([]DailyTraffic, error) {
	since := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	q := `SELECT t.day, SUM(t.bytes) FROM traffic_daily t`
	var where = []string{"t.day >= ?"}
	args := []any{since}
	if adminID != nil {
		q += ` JOIN users u ON u.id = t.user_id`
		where = append(where, "u.admin_id = ?")
		args = append(args, *adminID)
	}
	if userID != nil {
		where = append(where, "t.user_id = ?")
		args = append(args, *userID)
	}
	q += " WHERE " + strings.Join(where, " AND ") + " GROUP BY t.day ORDER BY t.day"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DailyTraffic{}
	for rows.Next() {
		var d DailyTraffic
		if err := rows.Scan(&d.Day, &d.Bytes); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
