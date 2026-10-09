package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

type ctxKey struct{}

func adminFrom(ctx context.Context) *model.Admin {
	a, _ := ctx.Value(ctxKey{}).(*model.Admin)
	return a
}

// ownerFilter returns the admin id to scope queries by, or nil for sudo admins.
func ownerFilter(a *model.Admin) *int64 {
	if a.Role == model.RoleSudo {
		return nil
	}
	return &a.ID
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		id, claims, err := s.issuer.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		a, err := s.store.GetAdmin(r.Context(), id)
		if err != nil || a.Disabled {
			writeError(w, http.StatusUnauthorized, "admin not found or disabled")
			return
		}
		// Tokens issued before a role change must not keep the old privileges.
		if claims.Role != a.Role {
			writeError(w, http.StatusUnauthorized, "session outdated, sign in again")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, a)))
	})
}

func requireSudo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := adminFrom(r.Context()); a == nil || a.Role != model.RoleSudo {
			writeError(w, http.StatusForbidden, "sudo admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) audit(r *http.Request, action, target, details string) {
	a := adminFrom(r.Context())
	if a == nil {
		return
	}
	e := &model.AuditEntry{AdminID: a.ID, Admin: a.Username, Action: action, Target: target, Details: details, IP: clientIP(r)}
	if err := s.store.AddAudit(r.Context(), e); err != nil {
		s.log.Warn("audit write failed", "err", err)
	}
}

func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 && !strings.HasSuffix(ip, "]") {
		ip = ip[:i]
	}
	return strings.Trim(ip, "[]")
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	Admin     *model.Admin `json:"admin"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	a, err := s.store.GetAdminByUsername(r.Context(), req.Username)
	if err != nil {
		auth.SpendCompareTime(req.Password)
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !auth.CheckPassword(a.PasswordHash, req.Password) || a.Disabled {
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.limiter.reset(ip)
	token, exp, err := s.issuer.Issue(a)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	ctx := context.WithValue(r.Context(), ctxKey{}, a)
	s.audit(r.WithContext(ctx), "login", a.Username, "")
	writeJSON(w, http.StatusOK, loginResponse{Token: token, ExpiresAt: exp, Admin: a})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, adminFrom(r.Context()))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &req) {
		return
	}
	a := adminFrom(r.Context())
	if !auth.CheckPassword(a.PasswordHash, req.Current) {
		writeError(w, http.StatusBadRequest, "current password is wrong")
		return
	}
	hash, err := auth.HashPassword(req.New)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.PasswordHash = hash
	if err := s.store.UpdateAdmin(r.Context(), a); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "admin.password", a.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

// loginLimiter blocks an IP after max failures within window.
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *loginLimiter) prune(ip string, now time.Time) []time.Time {
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, ip)
		return nil
	}
	l.hits[ip] = kept
	return kept
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(ip, time.Now())) < l.max
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.hits[ip] = append(l.prune(ip, now), now)
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, ip)
}
