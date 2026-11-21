package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/gdrive"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

// restoredUsers applies the staged backup at dbPath and lists the usernames
// in the restored database.
func restoredUsers(t *testing.T, dbPath string) []string {
	t.Helper()
	if done, err := ApplyPendingRestore(dbPath); err != nil || !done {
		t.Fatalf("apply restore: %v %v", done, err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	users, _, err := st.ListUsers(context.Background(), store.UserFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, u := range users {
		names = append(names, u.Username)
	}
	return names
}

// A downloaded backup restored on a panel brings back its data, and the
// panel restarts to apply it.
func TestBackupDownloadAndRestore(t *testing.T) {
	e := newEnv(t)
	dbPath := filepath.Join(t.TempDir(), "panel.db")
	e.api.cfg.DatabasePath = dbPath
	restarted := make(chan struct{}, 1)
	e.api.OnRestart = func() { restarted <- struct{}{} }
	root := e.login("root", "supersecret")
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, nil)

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/system/backup", nil)
	req.Header.Set("Authorization", "Bearer "+root)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.HasPrefix(backup, sqliteMagic) {
		t.Fatalf("download: %d %q", res.StatusCode, backup[:min(len(backup), 16)])
	}

	// later changes are not in the backup
	e.do("POST", "/api/users", root, userInput{Username: "bob"}, nil)

	upload := func(body []byte) int {
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/system/restore", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+root)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := upload([]byte("not a database")); code != 400 {
		t.Fatalf("garbage accepted: %d", code)
	}
	if _, err := os.Stat(dbPath + RestoreSuffix); err == nil {
		t.Fatal("garbage was staged")
	}
	if code := upload(backup); code != 200 {
		t.Fatalf("restore: %d", code)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the panel was not restarted")
	}
	if got := restoredUsers(t, dbPath); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("restored users: %v", got)
	}
}

// Scheduled backups go to Google Drive, old ones are pruned, and a new
// panel connected to the same Drive restores the latest.
func TestDriveBackups(t *testing.T) {
	e := newEnv(t)
	g, gs := newFakeGoogle()
	defer gs.Close()
	e.api.drive.client = gdrive.New(gdrive.Endpoints{OAuth: gs.URL + "/oauth", Drive: gs.URL + "/drive/v3", Upload: gs.URL + "/upload/drive/v3"})
	dbPath := filepath.Join(t.TempDir(), "panel.db")
	e.api.cfg.DatabasePath = dbPath
	e.api.OnRestart = func() {}
	root := e.login("root", "supersecret")
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, nil)

	var v driveBackupView
	if r := e.do("POST", "/api/system/drive-backups", root, nil, nil); r.StatusCode != 502 {
		t.Fatalf("backup before connecting: %d", r.StatusCode)
	}
	if err := e.api.drive.update(context.Background(), func(s *driveSettings) {
		s.ClientID, s.ClientSecret, s.RefreshToken = "cid", "secret", "refresh-1"
	}); err != nil {
		t.Fatal(err)
	}
	e.api.drive.client.Configure("cid", "secret", "refresh-1")
	if r := e.do("PUT", "/api/system/drive-backups", root, map[string]int{"hours": 12, "keep": 2}, &v); r.StatusCode != 200 || v.Hours != 12 || v.Keep != 2 {
		t.Fatalf("settings: %d %+v", r.StatusCode, v)
	}
	for i := 0; i < 3; i++ {
		v = driveBackupView{} // omitted fields must not carry over between answers
		if r := e.do("POST", "/api/system/drive-backups", root, nil, &v); r.StatusCode != 200 {
			t.Fatalf("backup %d: %d", i, r.StatusCode)
		}
	}
	if len(v.Files) != 2 || v.LastAt == nil || v.LastErr != "" {
		t.Fatalf("after three backups, keep 2: %+v", v)
	}
	g.mu.Lock()
	for id := range g.files {
		if !bytes.HasPrefix([]byte(g.files[id]), sqliteMagic) || g.public[id] {
			g.mu.Unlock()
			t.Fatalf("backup %s is not a private database", id)
		}
	}
	g.mu.Unlock()

	e.do("POST", "/api/users", root, userInput{Username: "bob"}, nil)
	if r := e.do("POST", "/api/system/drive-backups/"+v.Files[0].ID+"/restore", root, nil, nil); r.StatusCode != 200 {
		t.Fatalf("restore from drive: %d", r.StatusCode)
	}
	if got := restoredUsers(t, dbPath); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("restored users: %v", got)
	}
}
