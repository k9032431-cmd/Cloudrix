package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

func (s *Server) subURL(r *http.Request, u *model.User) string {
	base := s.cfg.SubPublicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + s.cfg.SubPath + "/" + u.SubToken
}

// endpoints resolves the inbounds a user can connect to; inactive users get none.
func (s *Server) endpoints(ctx context.Context, u *model.User) ([]sub.Endpoint, error) {
	if !u.Active() {
		return nil, nil
	}
	inbounds, err := s.store.ListInbounds(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	hosts := map[int64]string{}
	for _, n := range nodes {
		hosts[n.ID] = n.Address
	}
	return sub.Resolve(u, inbounds, hosts, s.cfg.DefaultHost), nil
}

// detectFormat picks the subscription format from ?format= or the client User-Agent.
func detectFormat(r *http.Request) string {
	if f := r.URL.Query().Get("format"); f != "" {
		return f
	}
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "clash"), strings.Contains(ua, "mihomo"), strings.Contains(ua, "stash"), strings.Contains(ua, "flclash"):
		return "clash"
	case strings.Contains(ua, "sing-box"), strings.Contains(ua, "sfa/"), strings.Contains(ua, "sfi/"), strings.Contains(ua, "sfm/"), strings.Contains(ua, "karing"):
		return "singbox"
	case strings.Contains(ua, "mozilla") && strings.Contains(r.Header.Get("Accept"), "text/html"):
		return "page"
	}
	return "base64"
}

func (s *Server) subscriptionUser(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	token := chi.URLParam(r, "token")
	if len(token) < 16 || len(token) > 64 {
		http.NotFound(w, r)
		return nil, false
	}
	u, err := s.store.GetUserBySubToken(r.Context(), token)
	if err != nil {
		if err != store.ErrNotFound {
			s.log.Error("subscription lookup", "err", err)
		}
		http.NotFound(w, r)
		return nil, false
	}
	return u, true
}

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	u, ok := s.subscriptionUser(w, r)
	if !ok {
		return
	}
	format := detectFormat(r)
	if format == "page" {
		s.serveSPA(w, r) // the web UI renders a subscription page for browsers
		return
	}

	if err := s.store.MarkSubscriptionFetched(r.Context(), u.ID, r.UserAgent()); err != nil {
		s.log.Warn("mark subscription fetched", "err", err)
	}

	eps, err := s.endpoints(r.Context(), u)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}

	// HWID device limit: clients such as Happ and v2RayTun send x-hwid.
	if hwid := r.Header.Get("x-hwid"); hwid != "" {
		allowed, err := s.store.TouchDevice(r.Context(), u.ID, store.Device{
			HWID: hwid, Platform: r.Header.Get("x-device-os"), OSVersion: r.Header.Get("x-ver-os"),
			DeviceModel: r.Header.Get("x-device-model"), UserAgent: r.UserAgent(),
		}, u.DeviceLimit)
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		if !allowed {
			eps = nil
			w.Header().Set("Announce", b64Header("Device limit reached"))
		}
	} else if u.DeviceLimit > 0 {
		// Without an HWID the limit cannot be enforced; refuse rather than leak.
		eps = nil
		w.Header().Set("Announce", b64Header("This client does not report a device ID"))
	}

	s.writeSubHeaders(w, u)
	switch format {
	case "clash":
		body, err := sub.Clash(u, eps)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "render failed")
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		_, _ = w.Write(body)
	case "singbox":
		body, err := sub.SingBox(u, eps)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "render failed")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	case "links":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(strings.Join(sub.Links(u, eps), "\n")))
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(sub.Base64Links(u, eps)))
	}
}

func (s *Server) writeSubHeaders(w http.ResponseWriter, u *model.User) {
	h := w.Header()
	b := s.branding()
	var expire int64
	if u.ExpireAt != nil {
		expire = u.ExpireAt.Unix()
	}
	h.Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d", u.UsedTraffic, u.DataLimit, expire))
	h.Set("Profile-Title", b64Header(b.Title))
	h.Set("Profile-Update-Interval", fmt.Sprint(b.UpdateHours))
	// A device-limit warning set earlier takes priority over the admin's announcement.
	if h.Get("Announce") == "" {
		if a := b.userAnnounce(u); a != "" {
			h.Set("Announce", b64Header(a))
		}
	}
	if b.SupportURL != "" {
		h.Set("Support-Url", b.SupportURL)
	}
	if b.WebPageURL != "" {
		h.Set("Profile-Web-Page-Url", b.WebPageURL)
	}
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, u.Username))
	h.Set("Cache-Control", "no-store")
}

type subscriptionInfo struct {
	Username      string              `json:"username"`
	Status        model.UserStatus    `json:"status"`
	DataLimit     int64               `json:"data_limit"`
	UsedTraffic   int64               `json:"used_traffic"`
	ExpireAt      *time.Time          `json:"expire_at"`
	ResetStrategy model.ResetStrategy `json:"reset_strategy"`
	URL           string              `json:"url"`
	DriveURL      string              `json:"gdrive_url,omitempty"` // backup link through googleapis.com
	Title         string              `json:"title"`
	Announce      string              `json:"announce,omitempty"`
	SupportURL    string              `json:"support_url,omitempty"`
	Links         []string            `json:"links"`
	WireGuard     []string            `json:"wireguard"` // inbound tags with downloadable .conf
}

// handleSubscriptionInfo powers the public subscription page.
func (s *Server) handleSubscriptionInfo(w http.ResponseWriter, r *http.Request) {
	u, ok := s.subscriptionUser(w, r)
	if !ok {
		return
	}
	eps, err := s.endpoints(r.Context(), u)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	info := subscriptionInfo{
		Username: u.Username, Status: u.Status, DataLimit: u.DataLimit, UsedTraffic: u.UsedTraffic,
		ExpireAt: u.ExpireAt, ResetStrategy: u.ResetStrategy, URL: s.subURL(r, u), Links: []string{}, WireGuard: []string{},
	}
	b := s.branding()
	info.Title, info.Announce, info.SupportURL = b.Title, b.userAnnounce(u), b.SupportURL
	if d := s.userDrive(r.Context(), u); d.URL != "" && u.DeviceLimit == 0 {
		info.DriveURL = d.URL
	}
	// With a device limit, links would bypass HWID checks, so only the URL is shown.
	if u.DeviceLimit == 0 {
		info.Links = append(info.Links, sub.Links(u, eps)...)
		for _, ep := range eps {
			if ep.Inbound.Protocol == model.ProtoWireGuard {
				info.WireGuard = append(info.WireGuard, ep.Inbound.Tag)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleWireGuardConf(w http.ResponseWriter, r *http.Request) {
	u, ok := s.subscriptionUser(w, r)
	if !ok {
		return
	}
	if u.DeviceLimit > 0 {
		http.NotFound(w, r)
		return
	}
	eps, err := s.endpoints(r.Context(), u)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	tag := strings.TrimSuffix(chi.URLParam(r, "tag"), ".conf")
	for _, ep := range eps {
		if ep.Inbound.Tag == tag && ep.Inbound.Protocol == model.ProtoWireGuard {
			conf, err := sub.WireGuardConf(u, ep)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.conf"`, u.Username, tag))
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(conf))
			return
		}
	}
	http.NotFound(w, r)
}
