package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/gdrive"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

const driveSettingsKey = "gdrive"

var errDriveDeviceLimit = errors.New("Google Drive links cannot check devices (HWID); remove the user's device limit to use them")

type driveSettings struct {
	Enabled         bool   `json:"enabled"`
	APIKey          string `json:"api_key"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	RefreshToken    string `json:"refresh_token"`
	Account         string `json:"account"`
	FolderID        string `json:"folder_id"`
	Format          string `json:"format"` // base64 | plain
	IntervalMinutes int    `json:"interval_minutes"`
}

func (d *driveSettings) normalize() {
	if d.Format != "plain" {
		d.Format = "base64"
	}
	if d.IntervalMinutes <= 0 {
		d.IntervalMinutes = 10
	}
}

type driveConnect struct {
	Pending         bool      `json:"pending"`
	UserCode        string    `json:"user_code,omitempty"`
	VerificationURL string    `json:"verification_url,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
	Error           string    `json:"error,omitempty"`
}

// driveSyncer mirrors user subscriptions to public Google Drive files.
type driveSyncer struct {
	s       *Server
	client  *gdrive.Client
	trigger chan struct{}

	mu     sync.Mutex // guards set and conn
	set    driveSettings
	conn   driveConnect
	cancel context.CancelFunc // running device-flow poller
	opMu   sync.Mutex         // serializes Drive writes
}

func newDriveSyncer(s *Server, ep gdrive.Endpoints) *driveSyncer {
	return &driveSyncer{s: s, client: gdrive.New(ep), trigger: make(chan struct{}, 1)}
}

func (d *driveSyncer) load(ctx context.Context) error {
	raw, err := d.s.store.GetSetting(ctx, driveSettingsKey)
	if errors.Is(err, store.ErrNotFound) {
		d.mu.Lock()
		d.set.normalize()
		d.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	var set driveSettings
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return err
	}
	set.normalize()
	d.mu.Lock()
	d.set = set
	d.mu.Unlock()
	d.client.Configure(set.ClientID, set.ClientSecret, set.RefreshToken)
	return nil
}

func (d *driveSyncer) settings() driveSettings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.set
}

// update applies fn to the settings and persists them.
func (d *driveSyncer) update(ctx context.Context, fn func(*driveSettings)) error {
	d.mu.Lock()
	set := d.set
	fn(&set)
	set.normalize()
	d.set = set
	d.mu.Unlock()
	d.client.Configure(set.ClientID, set.ClientSecret, set.RefreshToken)
	raw, _ := json.Marshal(set)
	return d.s.store.SetSetting(ctx, driveSettingsKey, string(raw))
}

// ready reports whether files can be written and links handed out.
func (d *driveSyncer) ready() bool {
	set := d.settings()
	return set.Enabled && set.APIKey != "" && set.RefreshToken != ""
}

func (d *driveSyncer) url(fileID string) string {
	return d.client.PublicURL(fileID, d.settings().APIKey)
}

func (d *driveSyncer) Trigger() {
	select {
	case d.trigger <- struct{}{}:
	default:
	}
}

func (d *driveSyncer) run(ctx context.Context) {
	for {
		interval := time.Duration(d.settings().IntervalMinutes) * time.Minute
		select {
		case <-ctx.Done():
			return
		case <-d.trigger:
			// Coalesce bursts of edits into one pass.
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		case <-time.After(interval):
		}
		if !d.ready() {
			continue
		}
		if err := d.syncAll(ctx); err != nil {
			d.s.log.Warn("google drive sync failed", "err", err)
		}
	}
}

// render produces the file body clients download from Drive. Drive cannot
// send subscription headers, so the plain format carries them as comments.
func (d *driveSyncer) render(ctx context.Context, u *model.User) ([]byte, error) {
	eps, err := d.s.endpoints(ctx, u)
	if err != nil {
		return nil, err
	}
	links := sub.Links(u, eps)
	if d.settings().Format == "base64" {
		return []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))), nil
	}
	var expire int64
	if u.ExpireAt != nil {
		expire = u.ExpireAt.Unix()
	}
	var b strings.Builder
	brand := d.s.branding()
	fmt.Fprintf(&b, "#profile-title: %s\n", b64Header(brand.Title))
	fmt.Fprintf(&b, "#profile-update-interval: %d\n", brand.UpdateHours)
	if a := brand.userAnnounce(u); a != "" {
		fmt.Fprintf(&b, "#announce: %s\n", b64Header(a))
	}
	if brand.SupportURL != "" {
		fmt.Fprintf(&b, "#support-url: %s\n", brand.SupportURL)
	}
	fmt.Fprintf(&b, "#subscription-userinfo: upload=0; download=%d; total=%d; expire=%d\n", u.UsedTraffic, u.DataLimit, expire)
	for _, l := range links {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// syncUser uploads the user's current subscription. Without create it only
// refreshes users that already have a Drive file.
func (d *driveSyncer) syncUser(ctx context.Context, u *model.User, create bool) (*store.DriveFile, error) {
	d.opMu.Lock()
	defer d.opMu.Unlock()

	rec, err := d.s.store.GetDriveFile(ctx, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		if !create {
			return nil, nil
		}
		rec = &store.DriveFile{UserID: u.ID}
	} else if err != nil {
		return nil, err
	}

	if u.DeviceLimit > 0 {
		if rec.FileID != "" {
			if err := d.client.Delete(ctx, rec.FileID); err != nil {
				d.s.log.Warn("delete drive file", "user", u.Username, "err", err)
			}
		}
		if err := d.s.store.DeleteDriveFile(ctx, u.ID); err != nil {
			return nil, err
		}
		if create {
			return nil, errDriveDeviceLimit
		}
		return nil, nil
	}

	content, err := d.render(ctx, u)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append([]byte(d.settings().Format+"\n"), content...))
	hash := hex.EncodeToString(sum[:])
	if rec.FileID != "" && rec.Hash == hash && rec.Error == "" {
		return rec, nil
	}

	upErr := d.upload(ctx, rec, content)
	now := time.Now().UTC()
	rec.SyncedAt = &now
	if upErr != nil {
		rec.Error = upErr.Error()
	} else {
		rec.Error, rec.Hash = "", hash
	}
	if rec.FileID != "" || upErr == nil {
		if err := d.s.store.SaveDriveFile(ctx, rec); err != nil {
			return nil, err
		}
	}
	return rec, upErr
}

func (d *driveSyncer) upload(ctx context.Context, rec *store.DriveFile, content []byte) error {
	if rec.FileID != "" {
		err := d.client.UpdateFile(ctx, rec.FileID, content)
		if !gdrive.IsNotFound(err) {
			return err
		}
		// The file was removed from Drive by hand: publish a new one.
		d.s.log.Warn("drive file disappeared, creating a new one", "user_id", rec.UserID)
		rec.FileID = ""
	}
	folder, err := d.folder(ctx, false)
	if err != nil {
		return err
	}
	name := "cx-" + auth.RandomHex(8) + ".txt" // random: the name is visible to anyone with the link
	id, err := d.client.CreateFile(ctx, folder, name, content)
	if gdrive.IsNotFound(err) { // folder deleted
		if folder, err = d.folder(ctx, true); err != nil {
			return err
		}
		id, err = d.client.CreateFile(ctx, folder, name, content)
	}
	if err != nil {
		return err
	}
	if err := d.client.SharePublic(ctx, id); err != nil {
		_ = d.client.Delete(ctx, id)
		return fmt.Errorf("share file: %w", err)
	}
	rec.FileID = id
	return nil
}

func (d *driveSyncer) folder(ctx context.Context, recreate bool) (string, error) {
	if id := d.settings().FolderID; id != "" && !recreate {
		return id, nil
	}
	id, err := d.client.CreateFolder(ctx, "Cloudrix subscriptions")
	if err != nil {
		return "", err
	}
	return id, d.update(ctx, func(s *driveSettings) { s.FolderID = id })
}

func (d *driveSyncer) syncAll(ctx context.Context) error {
	recs, err := d.s.store.ListDriveFiles(ctx)
	if err != nil || len(recs) == 0 {
		return err
	}
	users, err := d.s.store.AllUsers(ctx)
	if err != nil {
		return err
	}
	byID := make(map[int64]*model.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	var failed int
	for _, rec := range recs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		u, ok := byID[rec.UserID]
		if !ok {
			continue // user rows cascade; nothing to do
		}
		if _, err := d.syncUser(ctx, u, false); err != nil {
			failed++
			d.s.log.Debug("drive sync user", "user", u.Username, "err", err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files failed to sync", failed, len(recs))
	}
	return nil
}

// forget deletes the user's Drive file in the background (before the user row goes away).
func (d *driveSyncer) forget(ctx context.Context, userID int64) {
	rec, err := d.s.store.GetDriveFile(ctx, userID)
	if err != nil || rec.FileID == "" || d.settings().RefreshToken == "" {
		return
	}
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.opMu.Lock()
		defer d.opMu.Unlock()
		if err := d.client.Delete(ctx, id); err != nil {
			d.s.log.Warn("delete drive file", "err", err)
		}
	}(rec.FileID)
}

// rotate replaces the user's Drive file so the old link stops working.
func (d *driveSyncer) rotate(ctx context.Context, u *model.User) error {
	rec, err := d.s.store.GetDriveFile(ctx, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	d.opMu.Lock()
	if rec.FileID != "" {
		if err := d.client.Delete(ctx, rec.FileID); err != nil {
			d.s.log.Warn("delete drive file", "err", err)
		}
	}
	err = d.s.store.DeleteDriveFile(ctx, u.ID)
	d.opMu.Unlock()
	if err != nil || !d.ready() {
		return err
	}
	_, err = d.syncUser(ctx, u, true)
	return err
}

// ---- device flow ----

func (d *driveSyncer) startConnect(ctx context.Context) (driveConnect, error) {
	set := d.settings()
	if set.ClientID == "" || set.ClientSecret == "" {
		return driveConnect{}, errors.New("save the OAuth client ID and secret first")
	}
	auth, err := d.client.StartDeviceAuth(ctx, set.ClientID)
	if err != nil {
		return driveConnect{}, err
	}
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(auth.ExpiresIn)*time.Second)
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
	}
	d.cancel = cancel
	d.conn = driveConnect{Pending: true, UserCode: auth.UserCode, VerificationURL: auth.VerificationURL,
		ExpiresAt: time.Now().Add(time.Duration(auth.ExpiresIn) * time.Second)}
	conn := d.conn
	d.mu.Unlock()

	go d.pollConnect(pollCtx, cancel, set.ClientID, set.ClientSecret, auth)
	return conn, nil
}

func (d *driveSyncer) pollConnect(ctx context.Context, cancel context.CancelFunc, clientID, secret string, a *gdrive.DeviceAuth) {
	defer cancel()
	interval := time.Duration(a.Interval) * time.Second
	finish := func(errMsg string) {
		d.mu.Lock()
		d.conn.Pending = false
		d.conn.Error = errMsg
		d.mu.Unlock()
	}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				finish("the code expired, start again")
			}
			return
		case <-time.After(interval):
		}
		refresh, err := d.client.PollDeviceAuth(ctx, clientID, secret, a.DeviceCode)
		if errors.Is(err, gdrive.ErrPending) {
			continue
		}
		if err != nil {
			finish(err.Error())
			return
		}
		saveCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		err = d.update(saveCtx, func(s *driveSettings) {
			s.RefreshToken, s.Account, s.FolderID = refresh, "", ""
		})
		if err == nil {
			if email, aerr := d.client.Account(saveCtx); aerr == nil && email != "" {
				err = d.update(saveCtx, func(s *driveSettings) { s.Account = email })
			}
		}
		done()
		if err != nil {
			finish(err.Error())
			return
		}
		d.mu.Lock()
		d.conn = driveConnect{}
		d.mu.Unlock()
		d.Trigger()
		return
	}
}

// selfTest publishes a file, downloads it anonymously with the API key and removes it.
func (d *driveSyncer) selfTest(ctx context.Context) error {
	set := d.settings()
	if set.APIKey == "" || set.RefreshToken == "" {
		return errors.New("connect Google Drive and set the API key first")
	}
	d.opMu.Lock()
	defer d.opMu.Unlock()
	folder, err := d.folder(ctx, false)
	if err != nil {
		return err
	}
	want := "cloudrix-test-" + auth.RandomHex(6)
	id, err := d.client.CreateFile(ctx, folder, want+".txt", []byte(want))
	if gdrive.IsNotFound(err) {
		if folder, err = d.folder(ctx, true); err == nil {
			id, err = d.client.CreateFile(ctx, folder, want+".txt", []byte(want))
		}
	}
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer func() { _ = d.client.Delete(context.Background(), id) }()
	if err := d.client.SharePublic(ctx, id); err != nil {
		return fmt.Errorf("share: %w", err)
	}
	got, err := d.client.FetchPublic(ctx, id, set.APIKey)
	if err != nil {
		return fmt.Errorf("download with API key (is the Drive API enabled for this key?): %w", err)
	}
	if string(got) != want {
		return errors.New("downloaded content does not match")
	}
	return nil
}

// ---- HTTP handlers ----

type driveView struct {
	Enabled         bool         `json:"enabled"`
	APIKey          string       `json:"api_key"`
	ClientID        string       `json:"client_id"`
	HasClientSecret bool         `json:"has_client_secret"`
	Connected       bool         `json:"connected"`
	Account         string       `json:"account"`
	Format          string       `json:"format"`
	IntervalMinutes int          `json:"interval_minutes"`
	Connect         driveConnect `json:"connect"`
	Files           int          `json:"files"`
	Errors          int          `json:"errors"`
}

func (s *Server) handleGetDrive(w http.ResponseWriter, r *http.Request) {
	set := s.drive.settings()
	s.drive.mu.Lock()
	conn := s.drive.conn
	s.drive.mu.Unlock()
	v := driveView{
		Enabled: set.Enabled, APIKey: set.APIKey, ClientID: set.ClientID, HasClientSecret: set.ClientSecret != "",
		Connected: set.RefreshToken != "", Account: set.Account, Format: set.Format, IntervalMinutes: set.IntervalMinutes, Connect: conn,
	}
	recs, err := s.store.ListDriveFiles(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	for _, rec := range recs {
		if rec.FileID != "" {
			v.Files++
		}
		if rec.Error != "" {
			v.Errors++
		}
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateDrive(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled         bool   `json:"enabled"`
		APIKey          string `json:"api_key"`
		ClientID        string `json:"client_id"`
		ClientSecret    string `json:"client_secret"` // empty keeps the stored one
		Format          string `json:"format"`
		IntervalMinutes int    `json:"interval_minutes"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.APIKey, in.ClientID, in.ClientSecret = strings.TrimSpace(in.APIKey), strings.TrimSpace(in.ClientID), strings.TrimSpace(in.ClientSecret)
	err := s.drive.update(r.Context(), func(set *driveSettings) {
		if in.ClientID != set.ClientID {
			// A different OAuth client cannot use the old refresh token.
			set.RefreshToken, set.Account, set.FolderID = "", "", ""
		}
		set.Enabled, set.APIKey, set.ClientID = in.Enabled, in.APIKey, in.ClientID
		if in.ClientSecret != "" {
			set.ClientSecret = in.ClientSecret
		}
		set.Format, set.IntervalMinutes = in.Format, min(max(in.IntervalMinutes, 1), 1440)
	})
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "settings.gdrive", "", "")
	s.drive.Trigger()
	s.handleGetDrive(w, r)
}

func (s *Server) handleDriveConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := s.drive.startConnect(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "settings.gdrive.connect", "", "")
	writeJSON(w, http.StatusOK, conn)
}

func (s *Server) handleDriveDisconnect(w http.ResponseWriter, r *http.Request) {
	s.drive.mu.Lock()
	if s.drive.cancel != nil {
		s.drive.cancel()
	}
	s.drive.conn = driveConnect{}
	s.drive.mu.Unlock()
	if err := s.drive.update(r.Context(), func(set *driveSettings) { set.RefreshToken, set.Account = "", "" }); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "settings.gdrive.disconnect", "", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDriveTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.drive.selfTest(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDriveSyncAll(w http.ResponseWriter, r *http.Request) {
	if !s.drive.ready() {
		writeError(w, http.StatusBadRequest, "google drive is not enabled or not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.drive.syncAll(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type userDriveView struct {
	Available bool       `json:"available"` // feature configured
	URL       string     `json:"url,omitempty"`
	SyncedAt  *time.Time `json:"synced_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func (s *Server) userDrive(ctx context.Context, u *model.User) userDriveView {
	v := userDriveView{Available: s.drive.ready()}
	if !v.Available {
		return v
	}
	if rec, err := s.store.GetDriveFile(ctx, u.ID); err == nil {
		if rec.FileID != "" {
			v.URL = s.drive.url(rec.FileID)
		}
		v.SyncedAt, v.Error = rec.SyncedAt, rec.Error
	}
	return v
}

// handleUserDrive creates (or refreshes) the user's Google Drive link.
func (s *Server) handleUserDrive(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	if !s.drive.ready() {
		writeError(w, http.StatusBadRequest, "google drive is not enabled or not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if _, err := s.drive.syncUser(ctx, u, true); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errDriveDeviceLimit) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	s.audit(r, "user.gdrive", u.Username, "")
	writeJSON(w, http.StatusOK, s.userDrive(r.Context(), u))
}

func (s *Server) handleDeleteUserDrive(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	s.drive.forget(r.Context(), u.ID)
	if err := s.store.DeleteDriveFile(r.Context(), u.ID); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.gdrive.delete", u.Username, "")
	w.WriteHeader(http.StatusNoContent)
}
