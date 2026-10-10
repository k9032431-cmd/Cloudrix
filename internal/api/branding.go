package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

const brandingKey = "subscription"

// branding is what client apps (Happ, v2RayTun, Streisand, …) show for the
// subscription: its title, an announcement and links. Admins edit it in Settings.
type branding struct {
	Title       string `json:"title"`
	Announce    string `json:"announce"` // supports user variables, see sub.RenderTemplate
	UpdateHours int    `json:"update_hours"`
	SupportURL  string `json:"support_url"`
	WebPageURL  string `json:"web_page_url"`
}

func (s *Server) defaultBranding() branding {
	return branding{Title: s.cfg.SubTitle, UpdateHours: s.cfg.SubUpdateHours}
}

func (s *Server) loadBranding(ctx context.Context) error {
	b := s.defaultBranding()
	raw, err := s.store.GetSetting(ctx, brandingKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			return err
		}
	}
	s.brandMu.Lock()
	s.brand = b
	s.brandMu.Unlock()
	return nil
}

func (s *Server) branding() branding {
	s.brandMu.RLock()
	defer s.brandMu.RUnlock()
	return s.brand
}

// userAnnounce renders the announcement for one user.
func (b branding) userAnnounce(u *model.User) string {
	return strings.TrimSpace(sub.RenderTemplate(b.Announce, u))
}

func b64Header(v string) string {
	return "base64:" + base64.StdEncoding.EncodeToString([]byte(v))
}

func validLink(v string) bool {
	if v == "" {
		return true
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" && u.Opaque == "" {
		return false
	}
	switch u.Scheme {
	case "https", "http", "tg":
		return true
	}
	return false
}

func (s *Server) handleGetBranding(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.branding())
}

func (s *Server) handleUpdateBranding(w http.ResponseWriter, r *http.Request) {
	var in branding
	if !decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Announce = strings.TrimSpace(strings.ReplaceAll(in.Announce, "\r\n", "\n"))
	in.SupportURL, in.WebPageURL = strings.TrimSpace(in.SupportURL), strings.TrimSpace(in.WebPageURL)
	switch {
	case in.Title == "" || utf8.RuneCountInString(in.Title) > 64:
		writeError(w, http.StatusBadRequest, "title must be 1-64 characters")
		return
	case utf8.RuneCountInString(in.Announce) > 1000:
		writeError(w, http.StatusBadRequest, "announcement is too long (max 1000 characters)")
		return
	case in.UpdateHours < 1 || in.UpdateHours > 168:
		writeError(w, http.StatusBadRequest, "update interval must be 1-168 hours")
		return
	case !validLink(in.SupportURL) || !validLink(in.WebPageURL):
		writeError(w, http.StatusBadRequest, "links must start with https://, http:// or tg://")
		return
	}
	raw, _ := json.Marshal(in)
	if err := s.store.SetSetting(r.Context(), brandingKey, string(raw)); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.brandMu.Lock()
	s.brand = in
	s.brandMu.Unlock()
	s.audit(r, "settings.subscription", "", "")
	s.drive.Trigger() // Drive copies carry the title and announcement too
	writeJSON(w, http.StatusOK, in)
}
