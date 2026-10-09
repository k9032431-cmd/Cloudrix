package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

const inboundCols = `id, tag, protocol, listen, port, address, remark, node_id, enabled, settings`

func scanInbound(row interface{ Scan(...any) error }) (*model.Inbound, error) {
	var in model.Inbound
	var node sql.NullInt64
	var settings string
	err := row.Scan(&in.ID, &in.Tag, &in.Protocol, &in.Listen, &in.Port, &in.Address, &in.Remark, &node, &in.Enabled, &settings)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if node.Valid {
		in.NodeID = &node.Int64
	}
	if err := json.Unmarshal([]byte(settings), &in.Settings); err != nil {
		return nil, err
	}
	return &in, nil
}

func nullID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

func (s *Store) CreateInbound(ctx context.Context, in *model.Inbound) error {
	settings, _ := json.Marshal(in.Settings)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO inbounds (tag, protocol, listen, port, address, remark, node_id, enabled, settings) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Tag, in.Protocol, in.Listen, in.Port, in.Address, in.Remark, nullID(in.NodeID), in.Enabled, string(settings))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	in.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateInbound(ctx context.Context, in *model.Inbound) error {
	settings, _ := json.Marshal(in.Settings)
	res, err := s.db.ExecContext(ctx,
		`UPDATE inbounds SET tag = ?, protocol = ?, listen = ?, port = ?, address = ?, remark = ?, node_id = ?, enabled = ?, settings = ? WHERE id = ?`,
		in.Tag, in.Protocol, in.Listen, in.Port, in.Address, in.Remark, nullID(in.NodeID), in.Enabled, string(settings), in.ID)
	return affected(res, err)
}

func (s *Store) DeleteInbound(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM inbounds WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) GetInbound(ctx context.Context, id int64) (*model.Inbound, error) {
	return scanInbound(s.db.QueryRowContext(ctx, `SELECT `+inboundCols+` FROM inbounds WHERE id = ?`, id))
}

func (s *Store) ListInbounds(ctx context.Context) ([]*model.Inbound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+inboundCols+` FROM inbounds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Inbound{}
	for rows.Next() {
		in, err := scanInbound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}
