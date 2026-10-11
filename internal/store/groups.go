package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// ListGroups returns every group with its inbounds and user count.
func (s *Store) ListGroups(ctx context.Context) ([]*model.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.name, g.disabled, g.created_at,
		(SELECT COUNT(*) FROM users u, json_each(u.group_ids) j WHERE j.value = g.id)
		FROM groups g ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	out := []*model.Group{}
	byID := map[int64]*model.Group{}
	for rows.Next() {
		g := &model.Group{InboundIDs: []int64{}}
		var created int64
		if err := rows.Scan(&g.ID, &g.Name, &g.Disabled, &created, &g.Users); err != nil {
			rows.Close()
			return nil, err
		}
		g.CreatedAt = fromUnix(created)
		out = append(out, g)
		byID[g.ID] = g
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links, err := s.db.QueryContext(ctx, `SELECT group_id, inbound_id FROM group_inbounds ORDER BY inbound_id`)
	if err != nil {
		return nil, err
	}
	defer links.Close()
	for links.Next() {
		var gid, iid int64
		if err := links.Scan(&gid, &iid); err != nil {
			return nil, err
		}
		if g := byID[gid]; g != nil {
			g.InboundIDs = append(g.InboundIDs, iid)
		}
	}
	return out, links.Err()
}

// GroupMap returns the groups keyed by id, for resolving user access.
func (s *Store) GroupMap(ctx context.Context) (model.Groups, error) {
	list, err := s.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	m := make(model.Groups, len(list))
	for _, g := range list {
		m[g.ID] = g
	}
	return m, nil
}

func (s *Store) GetGroup(ctx context.Context, id int64) (*model.Group, error) {
	list, err := s.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range list {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) CreateGroup(ctx context.Context, g *model.Group) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		g.CreatedAt = time.Now().UTC()
		res, err := tx.ExecContext(ctx, `INSERT INTO groups (name, disabled, created_at) VALUES (?, ?, ?)`, g.Name, g.Disabled, unix(g.CreatedAt))
		if isUnique(err) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		if g.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		return setGroupInbounds(ctx, tx, g.ID, g.InboundIDs)
	})
}

func (s *Store) UpdateGroup(ctx context.Context, g *model.Group) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE groups SET name = ?, disabled = ? WHERE id = ?`, g.Name, g.Disabled, g.ID)
		if isUnique(err) {
			return ErrConflict
		}
		if err := affected(res, err); err != nil {
			return err
		}
		return setGroupInbounds(ctx, tx, g.ID, g.InboundIDs)
	})
}

func setGroupInbounds(ctx context.Context, tx *sql.Tx, id int64, inbounds []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_inbounds WHERE group_id = ?`, id); err != nil {
		return err
	}
	for _, iid := range inbounds {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO group_inbounds (group_id, inbound_id) VALUES (?, ?)`, id, iid); err != nil {
			return err
		}
	}
	return nil
}

// DeleteGroup removes the group and takes it off every user.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT u.id, u.group_ids FROM users u, json_each(u.group_ids) j WHERE j.value = ?`, id)
		if err != nil {
			return err
		}
		next := map[int64][]int64{}
		for rows.Next() {
			var uid int64
			var raw string
			if err := rows.Scan(&uid, &raw); err != nil {
				rows.Close()
				return err
			}
			var ids []int64
			_ = json.Unmarshal([]byte(raw), &ids)
			next[uid] = slices.DeleteFunc(ids, func(x int64) bool { return x == id })
		}
		rows.Close()
		for uid, ids := range next {
			if ids == nil {
				ids = []int64{}
			}
			raw, _ := json.Marshal(ids)
			if _, err := tx.ExecContext(ctx, `UPDATE users SET group_ids = ? WHERE id = ?`, string(raw), uid); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetInboundGroups puts the inbound into exactly the given groups.
func (s *Store) SetInboundGroups(ctx context.Context, inboundID int64, groups []int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM group_inbounds WHERE inbound_id = ?`, inboundID); err != nil {
			return err
		}
		for _, gid := range groups {
			_, err := tx.ExecContext(ctx, `INSERT INTO group_inbounds (group_id, inbound_id) VALUES (?, ?)`, gid, inboundID)
			if err != nil && !isUnique(err) {
				return err
			}
		}
		return nil
	})
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return tx.Commit()
}
