package api

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// Device limit on the nodes. The HWID check only sees subscription
// downloads, which Google Drive links (and leaked keys) bypass. Nodes report
// the IPs each user is connected from right now; a user over the device
// limit for longer than deviceGrace loses their keys on every node for the
// block time set in the subscription settings, then gets them back.

var deviceGrace = 45 * time.Second // rides out Wi-Fi ↔ mobile switches; tests shorten it

const onlineFreshness = 90 * time.Second // older node reports are ignored

type nodeOnline struct {
	at    time.Time
	users map[int64][]string
}

type deviceGuard struct {
	mu      sync.Mutex
	nodes   map[int64]nodeOnline
	since   map[int64]time.Time // over the limit since
	blocked map[int64]time.Time // keys removed until
}

func newDeviceGuard() *deviceGuard {
	return &deviceGuard{nodes: map[int64]nodeOnline{}, since: map[int64]time.Time{}, blocked: map[int64]time.Time{}}
}

// report stores a node's view of who is online from where.
func (g *deviceGuard) report(nodeID int64, online map[string][]string) {
	users := map[int64][]string{}
	for email, ips := range online {
		if uid, ok := nodeproto.UserID(email); ok {
			users[uid] = append(users[uid], ips...)
		}
	}
	g.mu.Lock()
	g.nodes[nodeID] = nodeOnline{at: time.Now(), users: users}
	g.mu.Unlock()
}

// ips merges the fresh reports of all nodes: user id → distinct IPs.
func (g *deviceGuard) ips() map[int64][]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[int64][]string{}
	for _, n := range g.nodes {
		if time.Since(n.at) > onlineFreshness {
			continue
		}
		for uid, ips := range n.users {
			for _, ip := range ips {
				if !slices.Contains(out[uid], ip) {
					out[uid] = append(out[uid], ip)
				}
			}
		}
	}
	for _, list := range out {
		slices.Sort(list)
	}
	return out
}

// online returns the IPs a user is connected from and until when the user
// is blocked for going over the device limit.
func (g *deviceGuard) online(uid int64) ([]string, *time.Time) {
	ips := g.ips()[uid]
	g.mu.Lock()
	defer g.mu.Unlock()
	if until, ok := g.blocked[uid]; ok && time.Now().Before(until) {
		return ips, &until
	}
	return ips, nil
}

// blockedSet lists users whose keys are currently withheld from the nodes.
func (g *deviceGuard) blockedSet() map[int64]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[int64]bool{}
	for uid, until := range g.blocked {
		if time.Now().Before(until) {
			out[uid] = true
		}
	}
	return out
}

// checkDevices blocks users over their limit and releases expired blocks,
// pushing new configs to the nodes when either happens.
func (s *Server) checkDevices(ctx context.Context) {
	g := s.devices
	now := time.Now()
	minutes := s.branding().DeviceBlockMinutes
	ips := g.ips()

	changed := false
	var newly []int64
	g.mu.Lock()
	for uid, until := range g.blocked {
		if !now.Before(until) {
			delete(g.blocked, uid)
			changed = true
		}
	}
	for uid := range g.since {
		if _, still := ips[uid]; !still {
			delete(g.since, uid)
		}
	}
	g.mu.Unlock()

	for uid, list := range ips {
		u, err := s.store.GetUser(ctx, uid)
		if err != nil {
			continue
		}
		g.mu.Lock()
		_, isBlocked := g.blocked[uid]
		over := u.DeviceLimit > 0 && len(list) > u.DeviceLimit
		switch {
		case !over:
			delete(g.since, uid)
		case g.since[uid].IsZero():
			g.since[uid] = now
		case now.Sub(g.since[uid]) >= deviceGrace && !isBlocked && minutes > 0:
			g.blocked[uid] = now.Add(time.Duration(minutes) * time.Minute)
			delete(g.since, uid)
			newly = append(newly, uid)
			changed = true
		}
		g.mu.Unlock()
		if slices.Contains(newly, uid) {
			s.log.Info("device limit exceeded: keys withheld", "user", u.Username, "devices", len(list), "limit", u.DeviceLimit, "minutes", minutes)
			e := &model.AuditEntry{Admin: "system", Action: "user.device_limit", Target: u.Username,
				Details: fmt.Sprintf("%d devices online, limit %d: blocked for %d min (%v)", len(list), u.DeviceLimit, minutes, list)}
			if err := s.store.AddAudit(ctx, e); err != nil {
				s.log.Warn("audit write failed", "err", err)
			}
		}
	}
	if changed {
		s.usersChanged()
	}
}

// decorateUser adds the live device data the admin UI shows.
func (s *Server) decorateUser(u *model.User) {
	u.OnlineIPs, u.DeviceBlockedUntil = s.devices.online(u.ID)
}
