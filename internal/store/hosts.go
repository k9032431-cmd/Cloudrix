package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

const hostCols = `id, inbound_id, priority, enabled, data`

func scanHost(row interface{ Scan(...any) error }) (*model.Host, error) {
	var h model.Host
	var data string
	err := row.Scan(&h.ID, &h.InboundID, &h.Priority, &h.Enabled, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(data), &h.HostSettings); err != nil {
		return nil, err
	}
	return &h, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertHost adds a host; without a priority it goes to the end of the list.
func insertHost(ctx context.Context, db execer, h *model.Host) error {
	data, _ := json.Marshal(h.HostSettings)
	res, err := db.ExecContext(ctx, `INSERT INTO hosts (inbound_id, priority, enabled, data)
		VALUES (?, CASE WHEN ? > 0 THEN ? ELSE (SELECT COALESCE(MAX(priority), 0) + 1 FROM hosts) END, ?, ?)`,
		h.InboundID, h.Priority, h.Priority, h.Enabled, string(data))
	if err != nil {
		if isForeignKey(err) {
			return ErrNotFound
		}
		return err
	}
	h.ID, err = res.LastInsertId()
	return err
}

func (s *Store) CreateHost(ctx context.Context, h *model.Host) error {
	if err := insertHost(ctx, s.db, h); err != nil {
		return err
	}
	got, err := s.GetHost(ctx, h.ID)
	if err == nil {
		*h = *got
	}
	return err
}

func (s *Store) UpdateHost(ctx context.Context, h *model.Host) error {
	data, _ := json.Marshal(h.HostSettings)
	res, err := s.db.ExecContext(ctx, `UPDATE hosts SET inbound_id = ?, enabled = ?, data = ? WHERE id = ?`,
		h.InboundID, h.Enabled, string(data), h.ID)
	if isForeignKey(err) {
		return ErrNotFound
	}
	return affected(res, err)
}

func (s *Store) DeleteHost(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) GetHost(ctx context.Context, id int64) (*model.Host, error) {
	return scanHost(s.db.QueryRowContext(ctx, `SELECT `+hostCols+` FROM hosts WHERE id = ?`, id))
}

// ListHosts returns all hosts in subscription order.
func (s *Store) ListHosts(ctx context.Context) ([]*model.Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+hostCols+` FROM hosts ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ReorderHosts sets the subscription order: ids first to last. Hosts not
// listed keep their place after them.
func (s *Store) ReorderHosts(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE hosts SET priority = priority + ?`, len(ids)+1); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE hosts SET priority = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// attachHosts loads the hosts of the given inbounds (never nil after it).
func (s *Store) attachHosts(ctx context.Context, inbounds []*model.Inbound) error {
	byID := map[int64]*model.Inbound{}
	for _, in := range inbounds {
		in.Hosts = []model.Host{}
		byID[in.ID] = in
	}
	hosts, err := s.ListHosts(ctx)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if in := byID[h.InboundID]; in != nil {
			in.Hosts = append(in.Hosts, *h)
		}
	}
	return nil
}
