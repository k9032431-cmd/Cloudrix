package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

const nodeCols = `id, name, address, api_port, usage_coefficient, status, message, core_version, last_seen,
	upload_bytes, download_bytes, created_at`

func scanNode(row interface{ Scan(...any) error }) (*model.Node, error) {
	var n model.Node
	var lastSeen sql.NullInt64
	var created int64
	err := row.Scan(&n.ID, &n.Name, &n.Address, &n.APIPort, &n.UsageCoefficient, &n.Status, &n.Message, &n.CoreVersion,
		&lastSeen, &n.UploadBytes, &n.DownloadBytes, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n.LastSeen = ptrUnix(lastSeen)
	n.CreatedAt = fromUnix(created)
	return &n, nil
}

func (s *Store) CreateNode(ctx context.Context, n *model.Node) error {
	n.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO nodes (name, address, api_port, usage_coefficient, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		n.Name, n.Address, n.APIPort, n.UsageCoefficient, n.Status, unix(n.CreatedAt))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	n.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateNode(ctx context.Context, n *model.Node) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET name = ?, address = ?, api_port = ?, usage_coefficient = ?, status = ?, message = ?, core_version = ?,
		last_seen = ?, upload_bytes = ?, download_bytes = ? WHERE id = ?`,
		n.Name, n.Address, n.APIPort, n.UsageCoefficient, n.Status, n.Message, n.CoreVersion, nullUnix(n.LastSeen),
		n.UploadBytes, n.DownloadBytes, n.ID)
	return affected(res, err)
}

func (s *Store) DeleteNode(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) GetNode(ctx context.Context, id int64) (*model.Node, error) {
	return scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id))
}

func (s *Store) ListNodes(ctx context.Context) ([]*model.Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeCols+` FROM nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
