package api

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// adminHold tracks which admins are on hold (expired or over their traffic
// limit) so node configs change exactly when an admin crosses a limit.
type adminHold struct {
	mu   sync.Mutex
	last map[int64]bool
}

// suspendedAdmins lists the admins whose users must get no keys right now.
func (s *Server) suspendedAdmins(ctx context.Context) map[int64]bool {
	admins, err := s.store.ListAdmins(ctx)
	out := map[int64]bool{}
	if err != nil {
		return out
	}
	now := time.Now()
	for _, a := range admins {
		if a.Suspended(now) != "" {
			out[a.ID] = true
		}
	}
	return out
}

// checkAdmins pushes new configs when an admin's term ends or their users'
// traffic reaches the admin's limit (and again once it is raised).
func (s *Server) checkAdmins(ctx context.Context) {
	now := s.suspendedAdmins(ctx)
	s.adminHold.mu.Lock()
	changed := s.adminHold.last != nil && !maps.Equal(now, s.adminHold.last)
	s.adminHold.last = now
	s.adminHold.mu.Unlock()
	if changed {
		s.log.Info("admin limits changed which users are served", "on_hold", len(now))
		s.usersChanged()
	}
}

// checkUserCaps enforces an admin's caps on the users it creates or edits:
// each user needs a data limit and a device limit within them.
func checkUserCaps(a *model.Admin, in *userInput) error {
	if a == nil || a.Role == model.RoleSudo {
		return nil
	}
	if a.MaxUserData > 0 && (in.DataLimit <= 0 || in.DataLimit > a.MaxUserData) {
		return fmt.Errorf("traffic per user must be 1 B – %.1f GB for your account", float64(a.MaxUserData)/(1<<30))
	}
	if a.MaxUserDevices > 0 && (in.DeviceLimit <= 0 || in.DeviceLimit > a.MaxUserDevices) {
		return fmt.Errorf("device limit per user must be 1–%d for your account", a.MaxUserDevices)
	}
	return nil
}
