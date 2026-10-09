// Package api exposes the panel REST API, public subscriptions and the web UI.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/config"
	"github.com/k9032431-cmd/cloudrix/internal/gdrive"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	issuer  *auth.Issuer
	log     *slog.Logger
	web     fs.FS // built frontend; may be empty
	limiter *loginLimiter
	started time.Time
	drive   *driveSyncer
	// OnUsersChanged is called after mutations that affect core configs.
	OnUsersChanged func()
}

func New(cfg config.Config, st *store.Store, issuer *auth.Issuer, log *slog.Logger, web fs.FS) *Server {
	s := &Server{
		cfg: cfg, store: st, issuer: issuer, log: log, web: web,
		limiter: newLoginLimiter(5, 5*time.Minute),
		started: time.Now(),
	}
	s.drive = newDriveSyncer(s, gdrive.Google)
	return s
}

// Start loads persisted settings and runs background workers until ctx ends.
func (s *Server) Start(ctx context.Context) error {
	if err := s.drive.load(ctx); err != nil {
		return err
	}
	go s.drive.run(ctx)
	s.drive.Trigger()
	return nil
}

// NotifyUsersChanged lets other components (e.g. lifecycle jobs) report user changes.
func (s *Server) NotifyUsersChanged() { s.usersChanged() }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer, securityHeaders)
	if len(s.cfg.CORSOrigins) > 0 {
		r.Use(s.cors)
	}

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)
			r.Get("/auth/me", s.handleMe)
			r.Post("/auth/password", s.handleChangePassword)

			r.Get("/system/stats", s.handleStats)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Post("/users/bulk", s.handleBulkUsers)
			r.Get("/users/{id}", s.handleGetUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)
			r.Post("/users/{id}/reset-traffic", s.handleResetTraffic)
			r.Post("/users/{id}/revoke", s.handleRevokeUser)
			r.Get("/users/{id}/subscription", s.handleUserSubscription)
			r.Get("/users/{id}/devices", s.handleListDevices)
			r.Delete("/users/{id}/devices", s.handleDeleteDevice)
			r.Get("/users/{id}/traffic", s.handleUserTraffic)
			r.Post("/users/{id}/gdrive", s.handleUserDrive)
			r.Delete("/users/{id}/gdrive", s.handleDeleteUserDrive)

			r.Get("/inbounds", s.handleListInbounds)

			r.Group(func(r chi.Router) {
				r.Use(requireSudo)
				r.Get("/admins", s.handleListAdmins)
				r.Post("/admins", s.handleCreateAdmin)
				r.Put("/admins/{id}", s.handleUpdateAdmin)
				r.Delete("/admins/{id}", s.handleDeleteAdmin)

				r.Post("/inbounds", s.handleCreateInbound)
				r.Put("/inbounds/{id}", s.handleUpdateInbound)
				r.Delete("/inbounds/{id}", s.handleDeleteInbound)

				r.Get("/nodes", s.handleListNodes)
				r.Post("/nodes", s.handleCreateNode)
				r.Put("/nodes/{id}", s.handleUpdateNode)
				r.Delete("/nodes/{id}", s.handleDeleteNode)

				r.Get("/settings/gdrive", s.handleGetDrive)
				r.Put("/settings/gdrive", s.handleUpdateDrive)
				r.Post("/settings/gdrive/connect", s.handleDriveConnect)
				r.Post("/settings/gdrive/disconnect", s.handleDriveDisconnect)
				r.Post("/settings/gdrive/test", s.handleDriveTest)
				r.Post("/settings/gdrive/sync", s.handleDriveSyncAll)

				r.Get("/core/config", s.handleCoreConfig)
				r.Get("/audit", s.handleAudit)

				r.Post("/tools/reality-keys", s.handleRealityKeys)
				r.Post("/tools/wireguard-keys", s.handleWireGuardKeys)
				r.Post("/tools/random", s.handleRandom)
			})
		})
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) { writeError(w, http.StatusNotFound, "not found") })
	})

	r.Route(s.cfg.SubPath+"/{token}", func(r chi.Router) {
		r.Get("/", s.handleSubscription)
		r.Get("/info", s.handleSubscriptionInfo)
		r.Get("/wireguard/{tag}", s.handleWireGuardConf)
	})

	r.NotFound(s.serveSPA)
	return r
}

// serveSPA serves the built frontend and falls back to index.html for client routes.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if s.web == nil {
		http.Error(w, "web UI is not built; run `make web`", http.StatusNotFound)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p != "" {
		if f, err := s.web.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.web, p)
				return
			}
		}
	}
	index, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		http.Error(w, "web UI is not built; run `make web`", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, o := range s.cfg.CORSOrigins {
			if o == "*" || o == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Add("Vary", "Origin")
				break
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) usersChanged() {
	s.drive.Trigger()
	if s.OnUsersChanged != nil {
		s.OnUsersChanged()
	}
}
