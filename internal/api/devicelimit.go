package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// Device limit on the nodes. The HWID check only sees subscription
// downloads, which Google Drive links and shared keys bypass, so the nodes
// enforce the limit too: they report the IPs each user is connected from
// and when each last opened a connection. When a user has more devices
// online than the limit, the newest ones keep working and the oldest are
// cut off on every node for kickFor. A cut-off device that reconnects (VPN
// off and on) is the newest again, so it works and the next oldest goes.
// Only that user's traffic from that IP is blocked: other users behind the
// same carrier IP are not affected.

var kickFor = 30 * time.Second // long enough to drop the device's connections

const onlineFreshness = 90 * time.Second // older node reports are ignored

type nodeOnline struct {
	at    time.Time
	users map[int64]map[string]int64 // user id → IP → last connection (unix)
}

type deviceGuard struct {
	mu     sync.Mutex
	nodes  map[int64]nodeOnline
	emails map[int64]string               // user id → core email, as the nodes know it
	kicks  map[int64]map[string]time.Time // user id → IP → cut off until
}

func newDeviceGuard() *deviceGuard {
	return &deviceGuard{nodes: map[int64]nodeOnline{}, emails: map[int64]string{}, kicks: map[int64]map[string]time.Time{}}
}

// report stores a node's view of who is online from where.
func (g *deviceGuard) report(nodeID int64, online map[string]map[string]int64) {
	users := map[int64]map[string]int64{}
	g.mu.Lock()
	defer g.mu.Unlock()
	for email, ips := range online {
		if uid, ok := nodeproto.UserID(email); ok {
			users[uid] = ips
			g.emails[uid] = email
		}
	}
	g.nodes[nodeID] = nodeOnline{at: time.Now(), users: users}
}

// onlineByNode counts the users each node reports online now.
func (g *deviceGuard) onlineByNode() map[int64]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[int64]int{}
	for id, n := range g.nodes {
		if time.Since(n.at) <= onlineFreshness {
			out[id] = len(n.users)
		}
	}
	return out
}

// online merges the fresh reports of all nodes: user id → IP → last seen.
func (g *deviceGuard) onlineLocked() map[int64]map[string]int64 {
	out := map[int64]map[string]int64{}
	for _, n := range g.nodes {
		if time.Since(n.at) > onlineFreshness {
			continue
		}
		for uid, ips := range n.users {
			if out[uid] == nil {
				out[uid] = map[string]int64{}
			}
			for ip, ts := range ips {
				out[uid][ip] = max(out[uid][ip], ts)
			}
		}
	}
	return out
}

// pruneLocked drops expired kicks and reports whether any were dropped.
func (g *deviceGuard) pruneLocked(now time.Time) bool {
	changed := false
	for uid, ips := range g.kicks {
		for ip, until := range ips {
			if !now.Before(until) {
				delete(ips, ip)
				changed = true
			}
		}
		if len(ips) == 0 {
			delete(g.kicks, uid)
		}
	}
	return changed
}

// devices returns a user's devices online now and the ones cut off.
func (g *deviceGuard) devices(uid int64) (online, kicked []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for ip := range g.onlineLocked()[uid] {
		if until, ok := g.kicks[uid][ip]; ok && now.Before(until) {
			continue
		}
		online = append(online, ip)
	}
	for ip, until := range g.kicks[uid] {
		if now.Before(until) {
			kicked = append(kicked, ip)
		}
	}
	slices.Sort(online)
	slices.Sort(kicked)
	return online, kicked
}

// kickSet is what every node must block (core email → IPs) and its version.
func (g *deviceGuard) kickSet() (map[string][]string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	set := map[string][]string{}
	var keys []string
	for uid, ips := range g.kicks {
		email := g.emails[uid]
		if email == "" {
			continue
		}
		for ip, until := range ips {
			if now.Before(until) {
				set[email] = append(set[email], ip)
				keys = append(keys, email+" "+ip)
			}
		}
		slices.Sort(set[email])
	}
	if len(keys) == 0 {
		return nil, ""
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return set, hex.EncodeToString(sum[:8])
}

// checkDevices cuts off the oldest devices of users over their limit and
// lets expired cut-offs go, waking the nodes when the set changes.
func (s *Server) checkDevices(ctx context.Context) {
	g := s.devices
	now := time.Now()
	g.mu.Lock()
	changed := g.pruneLocked(now)
	online := g.onlineLocked()
	g.mu.Unlock()

	for uid, ips := range online {
		u, err := s.store.GetUser(ctx, uid)
		if err != nil || u.DeviceLimit <= 0 {
			continue
		}
		g.mu.Lock()
		type dev struct {
			ip string
			ts int64
		}
		var active []dev
		for ip, ts := range ips {
			if until, ok := g.kicks[uid][ip]; ok && now.Before(until) {
				continue // already cut off: its retries do not count
			}
			active = append(active, dev{ip, ts})
		}
		if len(active) > u.DeviceLimit {
			// newest first; the ones past the limit are the oldest
			sort.Slice(active, func(i, j int) bool {
				if active[i].ts != active[j].ts {
					return active[i].ts > active[j].ts
				}
				return active[i].ip < active[j].ip
			})
			if g.kicks[uid] == nil {
				g.kicks[uid] = map[string]time.Time{}
			}
			for _, d := range active[u.DeviceLimit:] {
				g.kicks[uid][d.ip] = now.Add(kickFor)
				s.log.Info("device limit: oldest device cut off", "user", u.Username, "ip", d.ip, "limit", u.DeviceLimit)
			}
			changed = true
		}
		g.mu.Unlock()
	}
	if changed {
		s.nodes.bump() // waiting nodes pick up the new set at once
	}
}

// watchDevices lets cut-offs expire on time even when no node reports.
func (s *Server) watchDevices(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkDevices(ctx)
		}
	}
}

// decorateUser adds the live device data the admin UI shows.
func (s *Server) decorateUser(u *model.User) {
	u.OnlineIPs, u.KickedIPs = s.devices.devices(u.ID)
}
