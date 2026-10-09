package jobs

import (
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestApply(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	t.Run("expires", func(t *testing.T) {
		u := &model.User{Status: model.StatusActive, ExpireAt: &past}
		if !Apply(u, now) || u.Status != model.StatusExpired {
			t.Fatalf("status = %s", u.Status)
		}
	})
	t.Run("limited", func(t *testing.T) {
		u := &model.User{Status: model.StatusActive, DataLimit: 100, UsedTraffic: 100}
		if !Apply(u, now) || u.Status != model.StatusLimited {
			t.Fatalf("status = %s", u.Status)
		}
	})
	t.Run("extended expired user reactivates", func(t *testing.T) {
		u := &model.User{Status: model.StatusExpired, ExpireAt: &future}
		if !Apply(u, now) || u.Status != model.StatusActive {
			t.Fatalf("status = %s", u.Status)
		}
	})
	t.Run("on hold starts on first connection", func(t *testing.T) {
		u := &model.User{Status: model.StatusOnHold, OnHoldDuration: 86400}
		if Apply(u, now) {
			t.Fatal("must not change before first connection")
		}
		u.OnlineAt = &now
		if !Apply(u, now) || u.Status != model.StatusActive || !u.ExpireAt.Equal(now.Add(24*time.Hour)) {
			t.Fatalf("status = %s expire = %v", u.Status, u.ExpireAt)
		}
	})
	t.Run("monthly reset lifts limit", func(t *testing.T) {
		u := &model.User{Status: model.StatusLimited, DataLimit: 100, UsedTraffic: 150, ResetStrategy: model.ResetMonth, LastResetAt: now.AddDate(0, -1, 0)}
		if !Apply(u, now) || u.Status != model.StatusActive || u.UsedTraffic != 0 {
			t.Fatalf("status = %s used = %d", u.Status, u.UsedTraffic)
		}
	})
	t.Run("disabled stays disabled", func(t *testing.T) {
		u := &model.User{Status: model.StatusDisabled, ExpireAt: &past}
		if Apply(u, now) {
			t.Fatal("disabled user must not change")
		}
	})
}
