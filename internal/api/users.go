package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/jobs"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.@-]{3,64}$`)

type userInput struct {
	Username       string              `json:"username"`
	Status         model.UserStatus    `json:"status"`
	DataLimit      int64               `json:"data_limit"`
	ExpireAt       *time.Time          `json:"expire_at"`
	OnHoldDuration int64               `json:"on_hold_duration"`
	ResetStrategy  model.ResetStrategy `json:"reset_strategy"`
	DeviceLimit    int                 `json:"device_limit"`
	Inbounds       []string            `json:"inbounds"`
	Note           string              `json:"note"`
	AdminID        *int64              `json:"admin_id"`
}

func (s *Server) validateUserInput(ctx context.Context, in *userInput) error {
	if !usernameRe.MatchString(in.Username) {
		return errors.New("username must be 3-64 chars: letters, digits, _ . @ -")
	}
	if in.Status == "" {
		in.Status = model.StatusActive
	}
	switch in.Status {
	case model.StatusActive, model.StatusDisabled:
	case model.StatusOnHold:
		if in.OnHoldDuration <= 0 {
			return errors.New("on_hold status requires on_hold_duration > 0")
		}
		in.ExpireAt = nil
	default:
		return errors.New("status must be active, disabled or on_hold")
	}
	if in.ResetStrategy == "" {
		in.ResetStrategy = model.ResetNone
	}
	if !in.ResetStrategy.Valid() {
		return errors.New("invalid reset_strategy")
	}
	if in.DataLimit < 0 || in.DeviceLimit < 0 || in.OnHoldDuration < 0 {
		return errors.New("limits must not be negative")
	}
	if len(in.Note) > 500 {
		return errors.New("note is too long")
	}
	if len(in.Inbounds) > 0 {
		all, err := s.store.ListInbounds(ctx)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, ib := range all {
			known[ib.Tag] = true
		}
		for _, t := range in.Inbounds {
			if !known[t] {
				return fmt.Errorf("unknown inbound %q", t)
			}
		}
	}
	return nil
}

func applyInput(u *model.User, in *userInput) {
	u.Username = in.Username
	u.Status = in.Status
	u.DataLimit = in.DataLimit
	u.ExpireAt = in.ExpireAt
	u.OnHoldDuration = in.OnHoldDuration
	u.ResetStrategy = in.ResetStrategy
	u.DeviceLimit = in.DeviceLimit
	u.Inbounds = in.Inbounds
	u.Note = in.Note
}

// ownedUser loads a user the current admin may manage; others look like 404.
func (s *Server) ownedUser(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	id, ok := idParam(w, r)
	if !ok {
		return nil, false
	}
	u, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return nil, false
	}
	a := adminFrom(r.Context())
	if a.Role != model.RoleSudo && u.AdminID != a.ID {
		writeError(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return u, true
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	a := adminFrom(r.Context())
	q := r.URL.Query()
	f := store.UserFilter{
		AdminID: ownerFilter(a),
		Search:  q.Get("search"),
		Status:  model.UserStatus(q.Get("status")),
		Sort:    q.Get("sort"),
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	}
	users, total, err := s.store.ListUsers(r.Context(), f)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "total": total})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.validateUserInput(r.Context(), &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a := adminFrom(r.Context())
	owner := a.ID
	if in.AdminID != nil && a.Role == model.RoleSudo {
		if _, err := s.store.GetAdmin(r.Context(), *in.AdminID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown admin_id")
			return
		}
		owner = *in.AdminID
	}
	if a.Role != model.RoleSudo && a.UserLimit > 0 {
		n, err := s.store.CountUsersByAdmin(r.Context(), a.ID)
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		if n >= a.UserLimit {
			writeError(w, http.StatusForbidden, fmt.Sprintf("user limit reached (%d)", a.UserLimit))
			return
		}
	}

	u := &model.User{SubToken: auth.RandomToken(24), Credentials: auth.NewCredentials(), AdminID: owner}
	applyInput(u, &in)
	jobs.Apply(u, time.Now().UTC())
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.create", u.Username, "")
	s.usersChanged(u.ID)
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	if u, ok := s.ownedUser(w, r); ok {
		writeJSON(w, http.StatusOK, u)
	}
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	var in userInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.validateUserInput(r.Context(), &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a := adminFrom(r.Context())
	if in.AdminID != nil && a.Role == model.RoleSudo && *in.AdminID != u.AdminID {
		if _, err := s.store.GetAdmin(r.Context(), *in.AdminID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown admin_id")
			return
		}
		u.AdminID = *in.AdminID
	}
	// Switching to on-hold restarts the timer: it begins on the next connection.
	if in.Status == model.StatusOnHold && u.Status != model.StatusOnHold {
		if err := s.store.ClearOnline(r.Context(), u.ID); err != nil {
			s.writeStoreError(w, err)
			return
		}
		u.OnlineAt = nil
	}
	applyInput(u, &in)
	jobs.Apply(u, time.Now().UTC())
	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.update", u.Username, "")
	s.usersChanged(u.ID)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	s.drive.forget(r.Context(), u.ID)
	if err := s.store.DeleteUser(r.Context(), u.ID); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.delete", u.Username, "")
	s.usersChanged(u.ID) // nothing to refresh; keeps core sync informed
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetTraffic(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	u.UsedTraffic = 0
	u.LastResetAt = time.Now().UTC()
	jobs.Apply(u, time.Now().UTC())
	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.reset_traffic", u.Username, "")
	s.usersChanged(u.ID)
	writeJSON(w, http.StatusOK, u)
}

// handleRevokeUser rotates the subscription token and all proxy credentials,
// cutting off every previously shared link and device.
func (s *Server) handleRevokeUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	u.SubToken = auth.RandomToken(24)
	u.Credentials = auth.NewCredentials()
	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteDevices(r.Context(), u.ID); err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.drive.rotate(r.Context(), u); err != nil {
		s.log.Warn("rotate drive link", "user", u.Username, "err", err)
	}
	s.audit(r, "user.revoke", u.Username, "")
	s.usersChanged(u.ID)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleUserSubscription(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	eps, err := s.endpoints(r.Context(), u)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	links := sub.Links(u, eps)
	if links == nil {
		links = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": s.subURL(r, u), "links": links, "gdrive": s.userDrive(r.Context(), u)})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	devices, err := s.store.ListDevices(r.Context(), u.ID)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

// handleDeleteDevice removes one device (?hwid=) or all devices of the user.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	var err error
	if hwid := r.URL.Query().Get("hwid"); hwid != "" {
		err = s.store.DeleteDevice(r.Context(), u.ID, hwid)
	} else {
		err = s.store.DeleteDevices(r.Context(), u.ID)
	}
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "user.devices.delete", u.Username, r.URL.Query().Get("hwid"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUserTraffic(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ownedUser(w, r)
	if !ok {
		return
	}
	days := min(max(queryInt(r, "days", 30), 1), 365)
	hist, err := s.store.TrafficHistory(r.Context(), &u.ID, nil, days)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

type bulkRequest struct {
	IDs     []int64 `json:"ids"`
	Action  string  `json:"action"` // delete | disable | enable | reset_traffic | extend | add_data
	Days    int     `json:"days"`
	AddData int64   `json:"add_data"`
}

func (s *Server) handleBulkUsers(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 1000 {
		writeError(w, http.StatusBadRequest, "ids must contain 1..1000 items")
		return
	}
	switch req.Action {
	case "delete", "disable", "enable", "reset_traffic":
	case "extend":
		if req.Days == 0 {
			writeError(w, http.StatusBadRequest, "days is required")
			return
		}
	case "add_data":
		if req.AddData == 0 {
			writeError(w, http.StatusBadRequest, "add_data is required")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}

	a := adminFrom(r.Context())
	now := time.Now().UTC()
	done := 0
	var changed []int64
	for _, id := range req.IDs {
		u, err := s.store.GetUser(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		if a.Role != model.RoleSudo && u.AdminID != a.ID {
			continue
		}
		if req.Action == "delete" {
			s.drive.forget(r.Context(), u.ID)
			err = s.store.DeleteUser(r.Context(), u.ID)
		} else {
			switch req.Action {
			case "disable":
				u.Status = model.StatusDisabled
			case "enable":
				if u.Status == model.StatusDisabled {
					u.Status = model.StatusActive
				}
			case "reset_traffic":
				u.UsedTraffic = 0
				u.LastResetAt = now
			case "extend":
				// Expired users are extended from now, active ones from their expiry.
				if u.ExpireAt != nil {
					base := *u.ExpireAt
					if base.Before(now) {
						base = now
					}
					exp := base.AddDate(0, 0, req.Days)
					u.ExpireAt = &exp
				}
			case "add_data":
				if u.DataLimit > 0 {
					u.DataLimit = max(u.DataLimit+req.AddData, 0)
				}
			}
			jobs.Apply(u, now)
			err = s.store.UpdateUser(r.Context(), u)
			changed = append(changed, u.ID)
		}
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		done++
	}
	s.audit(r, "user.bulk."+req.Action, fmt.Sprintf("%d users", done), "")
	s.usersChanged(changed...)
	writeJSON(w, http.StatusOK, map[string]int{"affected": done})
}
