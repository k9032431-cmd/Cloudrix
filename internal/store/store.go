// Package store persists panel state in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; one connection avoids SQLITE_BUSY churn.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

var migrations = []string{
	`CREATE TABLE admins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL,
		disabled INTEGER NOT NULL DEFAULT 0,
		user_limit INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		address TEXT NOT NULL,
		api_port INTEGER NOT NULL,
		usage_coefficient REAL NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		message TEXT NOT NULL DEFAULT '',
		core_version TEXT NOT NULL DEFAULT '',
		last_seen INTEGER,
		upload_bytes INTEGER NOT NULL DEFAULT 0,
		download_bytes INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE inbounds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tag TEXT NOT NULL UNIQUE,
		protocol TEXT NOT NULL,
		listen TEXT NOT NULL DEFAULT '0.0.0.0',
		port INTEGER NOT NULL,
		address TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		node_id INTEGER REFERENCES nodes(id) ON DELETE SET NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		settings TEXT NOT NULL DEFAULT '{}'
	);
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		sub_token TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL,
		data_limit INTEGER NOT NULL DEFAULT 0,
		used_traffic INTEGER NOT NULL DEFAULT 0,
		lifetime_used INTEGER NOT NULL DEFAULT 0,
		expire_at INTEGER,
		on_hold_duration INTEGER NOT NULL DEFAULT 0,
		reset_strategy TEXT NOT NULL DEFAULT 'no_reset',
		last_reset_at INTEGER NOT NULL,
		device_limit INTEGER NOT NULL DEFAULT 0,
		inbounds TEXT NOT NULL DEFAULT '[]',
		note TEXT NOT NULL DEFAULT '',
		admin_id INTEGER NOT NULL REFERENCES admins(id),
		credentials TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		online_at INTEGER,
		sub_updated_at INTEGER,
		sub_user_agent TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX users_admin ON users(admin_id);
	CREATE INDEX users_status ON users(status);
	CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		admin_id INTEGER NOT NULL,
		admin TEXT NOT NULL,
		action TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		details TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);
	CREATE INDEX audit_created ON audit_log(created_at);
	CREATE TABLE user_devices (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		hwid TEXT NOT NULL,
		platform TEXT NOT NULL DEFAULT '',
		os_version TEXT NOT NULL DEFAULT '',
		device_model TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		first_seen INTEGER NOT NULL,
		last_seen INTEGER NOT NULL,
		PRIMARY KEY (user_id, hwid)
	);
	CREATE TABLE traffic_daily (
		day TEXT NOT NULL,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		bytes INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (day, user_id)
	);`,
	// 2: subscriptions mirrored to Google Drive
	`CREATE TABLE user_gdrive (
		user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		file_id TEXT NOT NULL DEFAULT '',
		hash TEXT NOT NULL DEFAULT '',
		synced_at INTEGER,
		error TEXT NOT NULL DEFAULT ''
	);`,
	// 3: node agents authenticate with a per-node secret
	`ALTER TABLE nodes ADD COLUMN secret TEXT NOT NULL DEFAULT '';`,
	// 4: inbound groups; existing inbounds and "all inbounds" users join Default
	`CREATE TABLE groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		disabled INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE group_inbounds (
		group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		inbound_id INTEGER NOT NULL REFERENCES inbounds(id) ON DELETE CASCADE,
		PRIMARY KEY (group_id, inbound_id)
	);
	ALTER TABLE users ADD COLUMN group_ids TEXT NOT NULL DEFAULT '[]';
	INSERT INTO groups (name, created_at) VALUES ('Default', CAST(strftime('%s', 'now') AS INTEGER));
	INSERT INTO group_inbounds (group_id, inbound_id) SELECT (SELECT id FROM groups WHERE name = 'Default'), id FROM inbounds;
	UPDATE users SET group_ids = '[' || (SELECT id FROM groups WHERE name = 'Default') || ']' WHERE inbounds IN ('', '[]', 'null');`,
	// 5: fingerprint of the node's self-signed certificate, pinned by clients
	`ALTER TABLE nodes ADD COLUMN cert_sha256 TEXT NOT NULL DEFAULT '';`,
	// 6: cascade bridge (node-to-node VLESS + Reality) of exit nodes
	`ALTER TABLE nodes ADD COLUMN bridge TEXT NOT NULL DEFAULT '{}';`,
	// 7: admin limits (term, traffic of all their users, caps per user)
	`ALTER TABLE admins ADD COLUMN expire_at INTEGER;
	ALTER TABLE admins ADD COLUMN data_limit INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE admins ADD COLUMN used_traffic INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE admins ADD COLUMN max_user_data INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE admins ADD COLUMN max_user_devices INTEGER NOT NULL DEFAULT 0;
	UPDATE admins SET used_traffic = (SELECT COALESCE(SUM(lifetime_used), 0) FROM users WHERE users.admin_id = admins.id);`,
	// 8: traffic per node and day, and per user on each node
	`CREATE TABLE node_traffic_daily (
		day TEXT NOT NULL,
		node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		up INTEGER NOT NULL DEFAULT 0,
		down INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (day, node_id)
	);
	CREATE TABLE node_user_traffic_daily (
		day TEXT NOT NULL,
		node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		bytes INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (day, node_id, user_id)
	);
	CREATE INDEX node_user_traffic_node ON node_user_traffic_daily (node_id, day);`,
	// 9: hosts — the client links of inbounds; every inbound starts with one
	// made of its address, remark, SNI and fingerprint
	`CREATE TABLE hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		inbound_id INTEGER NOT NULL REFERENCES inbounds(id) ON DELETE CASCADE,
		priority INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		data TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX hosts_inbound ON hosts (inbound_id);
	INSERT INTO hosts (inbound_id, priority, enabled, data)
		SELECT id, id, 1, json_object(
			'remark', remark, 'address', address,
			'sni', COALESCE(json_extract(settings, '$.sni'), ''),
			'fingerprint', COALESCE(json_extract(settings, '$.fingerprint'), ''))
		FROM inbounds;
	UPDATE inbounds SET settings = json_set(settings, '$.reality_server_names', json_array(json_extract(settings, '$.sni')))
		WHERE json_extract(settings, '$.security') = 'reality'
		AND COALESCE(json_array_length(settings, '$.reality_server_names'), 0) = 0
		AND COALESCE(json_extract(settings, '$.sni'), '') != '';`,
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Time helpers: timestamps are stored as unix seconds.

func unix(t time.Time) int64 { return t.Unix() }

func fromUnix(v int64) time.Time { return time.Unix(v, 0).UTC() }

func nullUnix(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.Unix(), Valid: true}
}

func ptrUnix(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromUnix(v.Int64)
	return &t
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func isForeignKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// Backup writes a consistent copy of the database to path while the panel keeps running.
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
