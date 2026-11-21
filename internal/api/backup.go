package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/k9032431-cmd/cloudrix/internal/store"
)

// Moving the panel: a backup is the whole SQLite database (users, nodes and
// their keys, cores, hosts, settings, the Google Drive connection). It can be
// downloaded, kept in Google Drive automatically, and restored on a new
// panel from a file or from Drive. A restore is staged next to the database
// and applied by the next start (see ApplyPendingRestore), so the running
// panel never swaps its database under open connections.

// RestoreSuffix names the staged backup a restart puts in place.
const RestoreSuffix = ".restore"

// maxBackupSize bounds uploads and Drive downloads of a backup.
const maxBackupSize = 512 << 20

// driveBackupPrefix starts the names of backups kept in Google Drive.
const driveBackupPrefix = "cloudrix-backup-"

var sqliteMagic = []byte("SQLite format 3\x00")

// snapshot returns a consistent copy of the database.
func (s *Server) snapshot(ctx context.Context) ([]byte, error) {
	dir := filepath.Dir(s.cfg.DatabasePath)
	f, err := os.CreateTemp(dir, ".backup-*.db")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // VACUUM INTO wants a new file
	defer os.Remove(path)
	if err := s.store.Backup(ctx, path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func backupName(prefix string) string {
	return prefix + time.Now().UTC().Format("20060102-150405") + ".db"
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	data, err := s.snapshot(r.Context())
	if err != nil {
		s.log.Error("backup", "err", err)
		writeError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
		return
	}
	s.audit(r, "system.backup", "", "")
	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+backupName("cloudrix-")+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// stageRestore checks that data is a Cloudrix database and puts it where the
// next start picks it up.
func (s *Server) stageRestore(ctx context.Context, data []byte) error {
	if !bytes.HasPrefix(data, sqliteMagic) {
		return errors.New("this is not a Cloudrix backup (not an SQLite database)")
	}
	dir := filepath.Dir(s.cfg.DatabasePath)
	f, err := os.CreateTemp(dir, ".restore-*.db")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	// Opening runs the migrations, so a backup from an older version is
	// upgraded now and a broken file is refused before anything changes.
	st, err := store.Open(tmp)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("this backup cannot be opened: %w", err)
	}
	n, err := st.CountAdmins(ctx)
	if err == nil {
		_, _, err = st.Checkpoint(ctx, tmp)
	}
	st.Close()
	os.Remove(tmp + "-wal")
	os.Remove(tmp + "-shm")
	if err != nil || n == 0 {
		os.Remove(tmp)
		if err == nil {
			err = errors.New("the backup has no administrators")
		}
		return fmt.Errorf("this is not a usable Cloudrix backup: %w", err)
	}
	return os.Rename(tmp, s.cfg.DatabasePath+RestoreSuffix)
}

// restartSoon lets the response reach the browser, then asks main to restart
// the panel, which applies the staged backup.
func (s *Server) restartSoon() {
	if s.OnRestart == nil {
		return
	}
	go func() {
		time.Sleep(time.Second)
		s.OnRestart()
	}()
}

func (s *Server) handleRestoreUpload(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBackupSize))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "the backup is too large or the upload was cut off")
		return
	}
	if err := s.stageRestore(r.Context(), data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "system.restore", "file", fmt.Sprint(len(data)))
	writeJSON(w, http.StatusOK, map[string]bool{"restarting": true})
	s.restartSoon()
}

// ---- Google Drive ----

type driveBackupView struct {
	Connected bool        `json:"connected"`
	Hours     int         `json:"hours"`
	Keep      int         `json:"keep"`
	LastAt    *time.Time  `json:"last_at,omitempty"`
	LastErr   string      `json:"last_error,omitempty"`
	Files     []driveFile `json:"files"`
	Error     string      `json:"error,omitempty"` // listing failed
}

type driveFile struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Size    int64     `json:"size"`
}

// listBackups returns the backups this OAuth client keeps in Drive, wherever
// they are: a new panel restores what the old one uploaded.
func (d *driveSyncer) listBackups(ctx context.Context) ([]driveFile, error) {
	files, err := d.client.List(ctx, "name contains '"+driveBackupPrefix+"' and trashed = false")
	if err != nil {
		return nil, err
	}
	out := make([]driveFile, 0, len(files))
	for _, f := range files {
		if strings.HasPrefix(f.Name, driveBackupPrefix) {
			out = append(out, driveFile{ID: f.ID, Name: f.Name, Created: f.Created, Size: f.Size})
		}
	}
	return out, nil
}

// backupNow uploads a snapshot to the backups folder and drops the oldest
// beyond the number to keep.
func (d *driveSyncer) backupNow(ctx context.Context) error {
	err := d.backup(ctx)
	now := time.Now()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if uerr := d.update(ctx, func(s *driveSettings) {
		s.BackupError = msg
		if err == nil {
			s.BackupAt = &now
		}
	}); uerr != nil && err == nil {
		err = uerr
	}
	return err
}

func (d *driveSyncer) backup(ctx context.Context) error {
	if d.settings().RefreshToken == "" {
		return errors.New("connect Google Drive first")
	}
	data, err := d.s.snapshot(ctx)
	if err != nil {
		return err
	}
	folder := d.settings().BackupFolderID
	if folder == "" {
		if folder, err = d.client.CreateFolder(ctx, "Cloudrix backups"); err != nil {
			return err
		}
		if err := d.update(ctx, func(s *driveSettings) { s.BackupFolderID = folder }); err != nil {
			return err
		}
	}
	if _, err := d.client.Upload(ctx, folder, backupName(driveBackupPrefix), "application/x-sqlite3", data); err != nil {
		return err
	}
	files, err := d.listBackups(ctx)
	if err != nil {
		return nil // uploaded; pruning waits for the next run
	}
	for _, f := range files[min(len(files), d.settings().BackupKeep):] {
		if err := d.client.Delete(ctx, f.ID); err != nil {
			d.s.log.Warn("delete old drive backup", "file", f.Name, "err", err)
		}
	}
	return nil
}

// runBackups makes the scheduled Drive backups.
func (d *driveSyncer) runBackups(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		set := d.settings()
		if set.BackupHours <= 0 || set.RefreshToken == "" {
			continue
		}
		if set.BackupAt != nil && time.Since(*set.BackupAt) < time.Duration(set.BackupHours)*time.Hour {
			continue
		}
		if err := d.backupNow(ctx); err != nil {
			d.s.log.Warn("google drive backup failed", "err", err)
		}
	}
}

func (s *Server) handleDriveBackups(w http.ResponseWriter, r *http.Request) {
	set := s.drive.settings()
	v := driveBackupView{Connected: set.RefreshToken != "", Hours: set.BackupHours, Keep: set.BackupKeep, LastAt: set.BackupAt, LastErr: set.BackupError, Files: []driveFile{}}
	if set.RefreshToken != "" {
		files, err := s.drive.listBackups(r.Context())
		if err != nil {
			v.Error = err.Error()
		} else {
			v.Files = files
		}
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateDriveBackups(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hours int `json:"hours"`
		Keep  int `json:"keep"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Hours < 0 || in.Hours > 168 || in.Keep < 1 || in.Keep > 100 {
		writeError(w, http.StatusBadRequest, "backup interval must be 0-168 hours and copies to keep 1-100")
		return
	}
	if err := s.drive.update(r.Context(), func(set *driveSettings) { set.BackupHours, set.BackupKeep = in.Hours, in.Keep }); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "settings.backups", fmt.Sprint(in.Hours), fmt.Sprint(in.Keep))
	s.handleDriveBackups(w, r)
}

func (s *Server) handleDriveBackupNow(w http.ResponseWriter, r *http.Request) {
	if err := s.drive.backupNow(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "backup to Google Drive failed: "+err.Error())
		return
	}
	s.audit(r, "system.backup", "drive", "")
	s.handleDriveBackups(w, r)
}

func (s *Server) handleDriveRestore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "fileID")
	data, err := s.drive.client.Download(r.Context(), id, maxBackupSize)
	if err != nil {
		writeError(w, http.StatusBadGateway, "download from Google Drive failed: "+err.Error())
		return
	}
	if err := s.stageRestore(r.Context(), data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "system.restore", "drive", id)
	writeJSON(w, http.StatusOK, map[string]bool{"restarting": true})
	s.restartSoon()
}

// ApplyPendingRestore puts a staged backup in place of the database before
// the panel opens it. The replaced database is kept next to it.
func ApplyPendingRestore(dbPath string) (bool, error) {
	staged := dbPath + RestoreSuffix
	if _, err := os.Stat(staged); err != nil {
		return false, nil
	}
	if _, err := os.Stat(dbPath); err == nil {
		old := dbPath + ".before-restore-" + time.Now().UTC().Format("20060102-150405")
		if err := os.Rename(dbPath, old); err != nil {
			return false, err
		}
		// the write-ahead log belongs to the replaced database
		os.Rename(dbPath+"-wal", old+"-wal")
		os.Remove(dbPath + "-shm")
	}
	if err := os.Rename(staged, dbPath); err != nil {
		return false, err
	}
	return true, nil
}
