package store

import (
	"context"
	"time"
)

// AddNodeTraffic records what a node moved today: its raw up/down bytes and
// the bytes of each user on it (before the usage coefficient).
func (s *Store) AddNodeTraffic(ctx context.Context, nodeID, up, down int64, perUser map[int64]int64) error {
	if up <= 0 && down <= 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	day := time.Now().UTC().Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, `INSERT INTO node_traffic_daily (day, node_id, up, down) VALUES (?, ?, ?, ?)
		ON CONFLICT(day, node_id) DO UPDATE SET up = up + excluded.up, down = down + excluded.down`,
		day, nodeID, up, down); err != nil {
		return err
	}
	for uid, b := range perUser {
		if b <= 0 {
			continue
		}
		// users deleted meanwhile are skipped
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_user_traffic_daily (day, node_id, user_id, bytes)
			SELECT ?, ?, id, ? FROM users WHERE id = ?
			ON CONFLICT(day, node_id, user_id) DO UPDATE SET bytes = bytes + excluded.bytes`,
			day, nodeID, b, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneNodeTraffic drops per-user node traffic older than keepUsers days and
// node totals older than keepNodes days.
func (s *Store) PruneNodeTraffic(ctx context.Context, keepUsers, keepNodes int) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM node_user_traffic_daily WHERE day < ?`,
		now.AddDate(0, 0, -keepUsers).Format("2006-01-02")); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM node_traffic_daily WHERE day < ?`,
		now.AddDate(0, 0, -keepNodes).Format("2006-01-02"))
	return err
}

type NodeDaily struct {
	Day   string `json:"day"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Bytes int64  `json:"bytes"` // up + down
}

// NodeTrafficHistory returns the daily traffic of one node (nil: all nodes)
// over the last `days` days, oldest first; days without traffic are left out.
func (s *Store) NodeTrafficHistory(ctx context.Context, nodeID *int64, days int) ([]NodeDaily, error) {
	since := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	q := `SELECT day, SUM(up), SUM(down) FROM node_traffic_daily WHERE day >= ?`
	args := []any{since}
	if nodeID != nil {
		q += ` AND node_id = ?`
		args = append(args, *nodeID)
	}
	rows, err := s.db.QueryContext(ctx, q+` GROUP BY day ORDER BY day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeDaily{}
	for rows.Next() {
		var d NodeDaily
		if err := rows.Scan(&d.Day, &d.Up, &d.Down); err != nil {
			return nil, err
		}
		d.Bytes = d.Up + d.Down
		out = append(out, d)
	}
	return out, rows.Err()
}

type NodeTotal struct {
	NodeID int64 `json:"node_id"`
	Up     int64 `json:"up"`
	Down   int64 `json:"down"`
	Users  int   `json:"users"` // users who used the node in the period
}

// NodeTotals sums each node's traffic over the last `days` days.
func (s *Store) NodeTotals(ctx context.Context, days int) (map[int64]NodeTotal, error) {
	since := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	out := map[int64]NodeTotal{}
	rows, err := s.db.QueryContext(ctx, `SELECT node_id, SUM(up), SUM(down) FROM node_traffic_daily WHERE day >= ? GROUP BY node_id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t NodeTotal
		if err := rows.Scan(&t.NodeID, &t.Up, &t.Down); err != nil {
			return nil, err
		}
		out[t.NodeID] = t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	urows, err := s.db.QueryContext(ctx, `SELECT node_id, COUNT(DISTINCT user_id) FROM node_user_traffic_daily WHERE day >= ? GROUP BY node_id`, since)
	if err != nil {
		return nil, err
	}
	defer urows.Close()
	for urows.Next() {
		var id int64
		var n int
		if err := urows.Scan(&id, &n); err != nil {
			return nil, err
		}
		t := out[id]
		t.NodeID, t.Users = id, n
		out[id] = t
	}
	return out, urows.Err()
}

type NodeUserUsage struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Bytes    int64  `json:"bytes"`
}

// NodeTopUsers lists the users who moved the most through a node over the
// last `days` days.
func (s *Store) NodeTopUsers(ctx context.Context, nodeID int64, days, limit int) ([]NodeUserUsage, error) {
	since := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx, `SELECT t.user_id, u.username, SUM(t.bytes) AS b
		FROM node_user_traffic_daily t JOIN users u ON u.id = t.user_id
		WHERE t.node_id = ? AND t.day >= ? GROUP BY t.user_id ORDER BY b DESC LIMIT ?`, nodeID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeUserUsage{}
	for rows.Next() {
		var u NodeUserUsage
		if err := rows.Scan(&u.UserID, &u.Username, &u.Bytes); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
