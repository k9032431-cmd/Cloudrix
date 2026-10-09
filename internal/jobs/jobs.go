// Package jobs runs periodic user lifecycle tasks: expiry, data limits,
// on-hold activation and periodic traffic resets.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

type Runner struct {
	Store    *store.Store
	Interval time.Duration
	Log      *slog.Logger
	// OnChange is called after any user changed state, so cores can be resynced.
	OnChange func()
}

func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		if n, err := r.Tick(ctx, time.Now().UTC()); err != nil {
			r.Log.Error("user lifecycle job failed", "err", err)
		} else if n > 0 {
			r.Log.Info("user states updated", "count", n)
			if r.OnChange != nil {
				r.OnChange()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick applies lifecycle rules to every user and returns how many changed.
func (r *Runner) Tick(ctx context.Context, now time.Time) (int, error) {
	users, err := r.Store.AllUsers(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, u := range users {
		if Apply(u, now) {
			if err := r.Store.UpdateUser(ctx, u); err != nil {
				return changed, err
			}
			changed++
		}
	}
	return changed, nil
}

// Apply mutates u according to lifecycle rules and reports whether it changed.
func Apply(u *model.User, now time.Time) bool {
	changed := false

	if due := nextReset(u, now); due {
		u.UsedTraffic = 0
		u.LastResetAt = now
		if u.Status == model.StatusLimited {
			u.Status = model.StatusActive
		}
		changed = true
	}

	switch u.Status {
	case model.StatusOnHold:
		// The timer starts on first connection.
		if u.OnlineAt != nil {
			exp := u.OnlineAt.Add(time.Duration(u.OnHoldDuration) * time.Second)
			u.ExpireAt = &exp
			u.Status = model.StatusActive
			changed = true
		}
	case model.StatusActive:
		if u.ExpireAt != nil && !now.Before(*u.ExpireAt) {
			u.Status = model.StatusExpired
			changed = true
		} else if u.DataLimit > 0 && u.UsedTraffic >= u.DataLimit {
			u.Status = model.StatusLimited
			changed = true
		}
	case model.StatusLimited:
		if u.DataLimit == 0 || u.UsedTraffic < u.DataLimit {
			u.Status = model.StatusActive
			changed = true
		}
	case model.StatusExpired:
		if u.ExpireAt == nil || now.Before(*u.ExpireAt) {
			u.Status = model.StatusActive
			changed = true
		}
	}
	return changed
}

func nextReset(u *model.User, now time.Time) bool {
	var next time.Time
	switch u.ResetStrategy {
	case model.ResetDay:
		next = u.LastResetAt.AddDate(0, 0, 1)
	case model.ResetWeek:
		next = u.LastResetAt.AddDate(0, 0, 7)
	case model.ResetMonth:
		next = u.LastResetAt.AddDate(0, 1, 0)
	default:
		return false
	}
	return !now.Before(next)
}
