package store

import (
	"context"
	"time"
)

type Device struct {
	HWID        string    `json:"hwid"`
	Platform    string    `json:"platform"`
	OSVersion   string    `json:"os_version"`
	DeviceModel string    `json:"device_model"`
	UserAgent   string    `json:"user_agent"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// TouchDevice records a subscription fetch from a device. A new device is
// rejected (allowed = false) once the user already has limit devices; limit 0
// means unlimited.
func (s *Store) TouchDevice(ctx context.Context, userID int64, d Device, limit int) (allowed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := unix(time.Now())
	d.HWID = trunc(d.HWID)

	res, err := tx.ExecContext(ctx, `UPDATE user_devices SET last_seen = ?, user_agent = ?, platform = ?, os_version = ?, device_model = ?
		WHERE user_id = ? AND hwid = ?`, now, trunc(d.UserAgent), trunc(d.Platform), trunc(d.OSVersion), trunc(d.DeviceModel), userID, d.HWID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, tx.Commit()
	}
	if limit > 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_devices WHERE user_id = ?`, userID).Scan(&count); err != nil {
			return false, err
		}
		if count >= limit {
			return false, nil
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_devices (user_id, hwid, platform, os_version, device_model, user_agent, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, userID, trunc(d.HWID), trunc(d.Platform), trunc(d.OSVersion), trunc(d.DeviceModel), trunc(d.UserAgent), now, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) ListDevices(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hwid, platform, os_version, device_model, user_agent, first_seen, last_seen
		FROM user_devices WHERE user_id = ? ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var first, last int64
		if err := rows.Scan(&d.HWID, &d.Platform, &d.OSVersion, &d.DeviceModel, &d.UserAgent, &first, &last); err != nil {
			return nil, err
		}
		d.FirstSeen, d.LastSeen = fromUnix(first), fromUnix(last)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDevice(ctx context.Context, userID int64, hwid string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_devices WHERE user_id = ? AND hwid = ?`, userID, hwid)
	return affected(res, err)
}

func (s *Store) DeleteDevices(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_devices WHERE user_id = ?`, userID)
	return err
}

func trunc(s string) string {
	if len(s) > 256 {
		return s[:256]
	}
	return s
}
