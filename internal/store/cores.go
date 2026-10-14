package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// ErrDefaultCore refuses removing the default (first) core.
var ErrDefaultCore = errors.New("the default core cannot be deleted")

const coreCols = `c.id, c.name, c.created_at,
	(SELECT COUNT(*) FROM inbounds i WHERE i.core_id = c.id),
	(SELECT COUNT(*) FROM nodes n WHERE COALESCE(n.core_id, (SELECT MIN(id) FROM cores)) = c.id)`

func scanCore(row interface{ Scan(...any) error }) (*model.Core, error) {
	var c model.Core
	var created int64
	err := row.Scan(&c.ID, &c.Name, &created, &c.Inbounds, &c.Nodes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = fromUnix(created)
	return &c, nil
}

func (s *Store) ListCores(ctx context.Context) ([]*model.Core, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+coreCols+` FROM cores c ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Core{}
	for rows.Next() {
		c, err := scanCore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCore(ctx context.Context, id int64) (*model.Core, error) {
	return scanCore(s.db.QueryRowContext(ctx, `SELECT `+coreCols+` FROM cores c WHERE c.id = ?`, id))
}

// DefaultCore is the first core: nodes without a choice run it.
func (s *Store) DefaultCore(ctx context.Context) (int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MIN(id) FROM cores`).Scan(&id)
	if err == nil && !id.Valid {
		return 0, ErrNotFound
	}
	return id.Int64, err
}

func (s *Store) CreateCore(ctx context.Context, c *model.Core) error {
	c.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO cores (name, created_at) VALUES (?, ?)`, c.Name, unix(c.CreatedAt))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (s *Store) RenameCore(ctx context.Context, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE cores SET name = ? WHERE id = ?`, name, id)
	if isUnique(err) {
		return ErrConflict
	}
	return affected(res, err)
}

// DeleteCore removes a core with its inbounds and their hosts; its nodes
// fall back to the default core.
func (s *Store) DeleteCore(ctx context.Context, id int64) error {
	def, err := s.DefaultCore(ctx)
	if err != nil {
		return err
	}
	if id == def {
		return ErrDefaultCore
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM cores WHERE id = ?`, id)
	return affected(res, err)
}

// DuplicateCore copies a core with its inbounds (tags get a suffix) and
// their hosts.
func (s *Store) DuplicateCore(ctx context.Context, id int64) (*model.Core, error) {
	src, err := s.GetCore(ctx, id)
	if err != nil {
		return nil, err
	}
	inbounds, err := s.ListInbounds(ctx)
	if err != nil {
		return nil, err
	}
	tags := map[string]bool{}
	for _, in := range inbounds {
		tags[in.Tag] = true
	}
	free := func(base string) string {
		t := base + "-copy"
		for i := 2; tags[t]; i++ {
			t = fmt.Sprintf("%s-copy%d", base, i)
		}
		tags[t] = true
		return t
	}
	c := &model.Core{Name: src.Name + " (copy)"}
	for i := 2; ; i++ {
		if err = s.CreateCore(ctx, c); !errors.Is(err, ErrConflict) {
			break
		}
		c.Name = fmt.Sprintf("%s (copy %d)", src.Name, i)
	}
	if err != nil {
		return nil, err
	}
	for _, in := range inbounds {
		if in.CoreID != id {
			continue
		}
		cp := *in
		cp.ID, cp.Tag, cp.CoreID, cp.NodeID = 0, free(in.Tag), c.ID, nil
		hosts := in.Hosts
		if err := s.CreateInbound(ctx, &cp); err != nil {
			return nil, err
		}
		// the copy's own first host is replaced by copies of the source hosts
		for _, h := range cp.Hosts {
			_ = s.DeleteHost(ctx, h.ID)
		}
		for _, h := range hosts {
			h.ID, h.InboundID, h.Priority = 0, cp.ID, 0
			if err := insertHost(ctx, s.db, &h); err != nil {
				return nil, err
			}
		}
	}
	return s.GetCore(ctx, c.ID)
}
