package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

var tagRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

func validateInbound(in *model.Inbound) error {
	if !tagRe.MatchString(in.Tag) {
		return errors.New("tag must be 1-64 chars: letters, digits, _ . -")
	}
	if in.Tag == "api" {
		return errors.New(`tag "api" is reserved`)
	}
	if !in.Protocol.Valid() {
		return errors.New("unknown protocol")
	}
	if in.Port < 1 || in.Port > 65535 {
		return errors.New("port must be 1..65535")
	}
	if in.Listen == "" {
		in.Listen = "0.0.0.0"
	}
	s := &in.Settings
	switch s.Network {
	case "", "tcp", "ws", "grpc", "xhttp", "httpupgrade":
	default:
		return errors.New("network must be tcp, ws, grpc, xhttp or httpupgrade")
	}
	switch s.Security {
	case "", "none", "tls", "reality":
	default:
		return errors.New("security must be none, tls or reality")
	}
	if s.Security == "reality" {
		if in.Protocol != model.ProtoVLESS && in.Protocol != model.ProtoTrojan {
			return errors.New("reality is supported for vless and trojan only")
		}
		if s.RealityPrivateKey == "" || s.RealityPublicKey == "" || s.RealityDest == "" {
			return errors.New("reality requires reality_dest and a key pair")
		}
	}
	switch in.Protocol {
	case model.ProtoHysteria2, model.ProtoTUIC:
		if s.CertFile == "" || s.KeyFile == "" {
			return errors.New(string(in.Protocol) + " requires cert_file and key_file")
		}
	case model.ProtoWireGuard:
		if s.WGPrivateKey == "" || s.WGPublicKey == "" {
			return errors.New("wireguard requires a server key pair")
		}
	case model.ProtoShadowsocks:
		if strings.HasPrefix(s.SSMethod, "2022-") && s.SSServerKey == "" {
			return errors.New("shadowsocks 2022 requires ss_server_key")
		}
	}
	return nil
}

func (s *Server) handleListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	// Non-sudo admins only need tags to assign users; hide server secrets.
	if adminFrom(r.Context()).Role != model.RoleSudo {
		type brief struct {
			Tag      string         `json:"tag"`
			Protocol model.Protocol `json:"protocol"`
			Remark   string         `json:"remark"`
			Enabled  bool           `json:"enabled"`
		}
		out := make([]brief, 0, len(inbounds))
		for _, in := range inbounds {
			out = append(out, brief{in.Tag, in.Protocol, in.Remark, in.Enabled})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, inbounds)
}

func (s *Server) handleCreateInbound(w http.ResponseWriter, r *http.Request) {
	var in model.Inbound
	if !decode(w, r, &in) {
		return
	}
	if err := validateInbound(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.CreateInbound(r.Context(), &in); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "inbound.create", in.Tag, string(in.Protocol))
	s.usersChanged()
	writeJSON(w, http.StatusCreated, in)
}

func (s *Server) handleUpdateInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in model.Inbound
	if !decode(w, r, &in) {
		return
	}
	in.ID = id
	if err := validateInbound(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateInbound(r.Context(), &in); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "inbound.update", in.Tag, "")
	s.usersChanged()
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleDeleteInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	in, err := s.store.GetInbound(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteInbound(r.Context(), id); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "inbound.delete", in.Tag, "")
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}

// handleCoreConfig previews the generated server config: ?core=xray|sing-box&node=<id>.
func (s *Server) handleCoreConfig(w http.ResponseWriter, r *http.Request) {
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	users, err := s.store.AllUsers(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	var nodeID *int64
	if n := queryInt(r, "node", 0); n > 0 {
		id := int64(n)
		nodeID = &id
	}
	selected := core.ForNode(inbounds, nodeID)
	var body []byte
	switch r.URL.Query().Get("core") {
	case "", "xray":
		body, err = core.Xray(selected, users)
	case "sing-box":
		body, err = core.SingBox(selected, users)
	default:
		writeError(w, http.StatusBadRequest, "core must be xray or sing-box")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *Server) handleRealityKeys(w http.ResponseWriter, _ *http.Request) {
	priv, pub := auth.X25519KeyPair(base64.RawURLEncoding)
	writeJSON(w, http.StatusOK, map[string]any{
		"private_key": priv, "public_key": pub, "short_ids": []string{auth.RandomHex(8), auth.RandomHex(4)},
	})
}

func (s *Server) handleWireGuardKeys(w http.ResponseWriter, _ *http.Request) {
	priv, pub := auth.X25519KeyPair(base64.StdEncoding)
	writeJSON(w, http.StatusOK, map[string]string{"private_key": priv, "public_key": pub})
}

// handleRandom returns random secrets: ?bytes=32 as base64 (fits SS-2022 keys) plus a hex variant.
func (s *Server) handleRandom(w http.ResponseWriter, r *http.Request) {
	n := min(max(queryInt(r, "bytes", 32), 8), 64)
	b := auth.RandomBytes(n)
	writeJSON(w, http.StatusOK, map[string]string{
		"base64": base64.StdEncoding.EncodeToString(b),
		"hex":    auth.RandomHex(n),
		"uuid":   auth.NewUUID(),
	})
}
