package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointEmptiesLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		if err := s.SetSetting(ctx, "k", strings.Repeat("x", 10000)); err != nil {
			t.Fatal(err)
		}
	}
	busy, size, err := s.Checkpoint(ctx, path)
	if err != nil || busy || size != 0 {
		t.Fatalf("busy=%v size=%d err=%v", busy, size, err)
	}
}

// A reader holding an old snapshot keeps the log from being emptied.
func TestCheckpointReportsStuckReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	tx, err := other.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "k", strings.Repeat("x", 10000)); err != nil {
		t.Fatal(err)
	}
	busy, size, err := s.Checkpoint(ctx, path)
	if err != nil || !busy || size == 0 {
		t.Fatalf("busy=%v size=%d err=%v", busy, size, err)
	}
}
