// Package nodeagent runs on a node server: it pulls core configs from the
// panel, runs Xray and sing-box, and reports per-user traffic back.
package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// Config is read from the environment the installer writes.
type Config struct {
	PanelURL   string
	NodeID     int64
	Secret     string
	Dir        string // state, configs and the self-signed certificate
	XrayBin    string
	SingBoxBin string
	XrayAssets string // geoip.dat / geosite.dat
	Version    string // agent build
	Log        *slog.Logger
}

func ConfigFromEnv(version string, log *slog.Logger) (Config, error) {
	c := Config{
		PanelURL:   strings.TrimRight(os.Getenv("CLOUDRIX_PANEL_URL"), "/"),
		Secret:     os.Getenv("CLOUDRIX_NODE_SECRET"),
		Dir:        envOr("CLOUDRIX_NODE_DIR", "/var/lib/cloudrix-node"),
		XrayBin:    envOr("CLOUDRIX_XRAY_BIN", "/usr/local/lib/cloudrix-node/xray/xray"),
		SingBoxBin: envOr("CLOUDRIX_SINGBOX_BIN", "/usr/local/lib/cloudrix-node/sing-box"),
		XrayAssets: envOr("XRAY_LOCATION_ASSET", "/usr/local/lib/cloudrix-node/xray"),
		Version:    version,
		Log:        log,
	}
	id, err := strconv.ParseInt(os.Getenv("CLOUDRIX_NODE_ID"), 10, 64)
	if err != nil || id <= 0 || c.PanelURL == "" || c.Secret == "" {
		return c, errors.New("CLOUDRIX_PANEL_URL, CLOUDRIX_NODE_ID and CLOUDRIX_NODE_SECRET must be set")
	}
	c.NodeID = id
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type Agent struct {
	cfg     Config
	log     *slog.Logger
	client  *http.Client
	xray    *proc
	singbox *proc
	fw      *firewall

	mu      sync.Mutex
	lastErr string

	version string
	pending map[string]*nodeproto.Stat
	lastTS  int64
	cores   string
	certPin string

	kicks    string // version of the device cut-offs in effect
	kicksGen int    // xray generation they were applied to
}

func New(cfg Config) *Agent {
	a := &Agent{
		cfg:     cfg,
		log:     cfg.Log,
		client:  &http.Client{Timeout: 60 * time.Second},
		pending: map[string]*nodeproto.Stat{},
		fw:      newFirewall(cfg.Log),
	}
	env := append(os.Environ(), "XRAY_LOCATION_ASSET="+cfg.XrayAssets)
	a.xray = newProc("xray", cfg.XrayBin, []string{"run", "-config", a.path("xray.json")}, env, cfg.Log, a.setErr)
	a.singbox = newProc("sing-box", cfg.SingBoxBin, []string{"run", "-c", a.path("sing-box.json")}, env, cfg.Log, a.setErr)
	return a
}

func (a *Agent) path(name string) string { return filepath.Join(a.cfg.Dir, name) }

func (a *Agent) setErr(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

func (a *Agent) takeErr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastErr
}

// Run keeps the node in sync with the panel until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.cfg.Dir, 0o700); err != nil {
		return err
	}
	if err := ensureSelfSigned(); err != nil {
		return fmt.Errorf("self-signed certificate: %w", err)
	}
	a.cores = coreVersions(a.cfg)
	if pin, err := certSHA256(core.SelfSignedCert); err == nil {
		a.certPin = pin
	} else {
		a.log.Warn("self-signed certificate hash", "err", err)
	}
	a.log.Info("cloudrix node agent", "panel", a.cfg.PanelURL, "node", a.cfg.NodeID, "cores", a.cores)
	a.resume()
	defer a.stopAll()

	backoff := 2 * time.Second
	for ctx.Err() == nil {
		a.collectStats(ctx)
		errText := a.takeErr()
		if p := a.probeExits(); p != "" {
			errText = strings.TrimPrefix(errText+"; "+p, "; ")
		}
		req := nodeproto.SyncRequest{Version: a.version, Cores: a.cores, Agent: a.cfg.Version, Error: errText, Wait: true, CertSHA256: a.certPin}
		req.Online, req.OnlineReported = a.onlineIPs(ctx)
		if a.kicksGen != a.xray.generation() {
			a.kicks = "" // a restarted xray lost the rules set through its API
		}
		req.Kicks = a.kicks
		req.Hysteria = a.hysteria()
		for _, st := range a.pending {
			req.Stats = append(req.Stats, *st)
		}
		resp, err := a.sync(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			a.log.Warn("panel unreachable", "err", err)
			a.writeState(false, err.Error())
			sleep(ctx, backoff)
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = 2 * time.Second
		a.pending = map[string]*nodeproto.Stat{} // delivered
		a.writeState(true, "")
		switch {
		case resp.Disabled:
			if a.version != "" || a.xray.running() || a.singbox.running() {
				a.log.Info("node disabled in the panel: stopping cores")
				a.stopAll()
				_ = os.Remove(a.path("xray.json"))
				_ = os.Remove(a.path("sing-box.json"))
				_ = os.Remove(a.path("version"))
				a.version = ""
			}
			sleep(ctx, 20*time.Second)
		case resp.Problem != "":
			a.setErr("panel could not build the config: " + resp.Problem)
			sleep(ctx, 15*time.Second)
		case resp.Changed:
			a.apply(ctx, resp)
			a.writeState(true, "")
		}
		if !resp.Disabled && resp.KicksVersion != a.kicks && a.xray.running() {
			if err := a.applyKicks(ctx, resp.Kicks); err != nil {
				a.log.Warn("device limit rules", "err", err)
				sleep(ctx, 5*time.Second)
			} else {
				a.kicks, a.kicksGen = resp.KicksVersion, a.xray.generation()
			}
		}
	}
	return nil
}

// resume starts the cores with the last applied configs so the node keeps
// serving clients even while the panel is unreachable.
func (a *Agent) resume() {
	if v, err := os.ReadFile(a.path("version")); err == nil {
		a.version = strings.TrimSpace(string(v))
	}
	if fileExists(a.path("xray.json")) {
		a.startOrWarn(a.xray)
	}
	if fileExists(a.path("sing-box.json")) {
		a.startOrWarn(a.singbox)
	}
}

func (a *Agent) startOrWarn(p *proc) {
	if err := p.start(); err != nil {
		a.setErr(fmt.Sprintf("%s: %v", p.name, err))
	}
}

func (a *Agent) stopAll() {
	a.xray.stop()
	a.singbox.stop()
}

func (a *Agent) sync(ctx context.Context, req nodeproto.SyncRequest) (*nodeproto.SyncResponse, error) {
	plain, _ := json.Marshal(req)
	body, err := nodeproto.Seal(a.cfg.Secret, "req", plain)
	if err != nil {
		return nil, err
	}
	ts := max(time.Now().UnixMilli(), a.lastTS+1)
	a.lastTS = ts
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.PanelURL+nodeproto.SyncPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/octet-stream")
	hr.Header.Set(nodeproto.HeaderNode, strconv.FormatInt(a.cfg.NodeID, 10))
	hr.Header.Set(nodeproto.HeaderTime, strconv.FormatInt(ts, 10))
	hr.Header.Set(nodeproto.HeaderSig, nodeproto.Sign(a.cfg.Secret, a.cfg.NodeID, ts, body))
	res, err := a.client.Do(hr)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("panel rejected the node key (401): reinstall the node with the command from the panel")
		}
		return nil, fmt.Errorf("panel answered %d: %s", res.StatusCode, strings.TrimSpace(string(data[:min(len(data), 200)])))
	}
	out, err := nodeproto.Open(a.cfg.Secret, "resp", data)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt the panel answer (wrong key?): %w", err)
	}
	var resp nodeproto.SyncResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// apply installs new core configs. A config that fails the core's own check
// is rejected and the running one stays; the version still advances so the
// agent waits for the next change instead of retrying in a loop.
func (a *Agent) apply(ctx context.Context, resp *nodeproto.SyncResponse) {
	var problems []string
	a.fw.allow(ports(resp.Xray, resp.SingBox))
	if err := a.applyXray(ctx, resp.Xray); err != nil {
		problems = append(problems, "xray: "+err.Error())
	}
	if err := a.applyCore(a.singbox, "sing-box.json", resp.SingBox, []string{"check", "-c"}); err != nil {
		problems = append(problems, "sing-box: "+err.Error())
	}
	a.version = resp.Version
	_ = os.WriteFile(a.path("version"), []byte(resp.Version), 0o600)
	a.setErr(strings.Join(problems, "; "))
	if len(problems) == 0 {
		a.log.Info("config applied", "version", resp.Version)
	} else {
		a.log.Warn("config rejected", "problems", problems)
	}
}

func (a *Agent) applyXray(ctx context.Context, next []byte) error {
	file := a.path("xray.json")
	old, _ := os.ReadFile(file)
	if len(next) == 0 {
		a.xray.stop()
		_ = os.Remove(file)
		return nil
	}
	if bytes.Equal(old, next) && a.xray.running() {
		return nil
	}
	if err := a.check(a.xray, next, []string{"run", "-test", "-config"}); err != nil {
		return err
	}
	// Only users changed: add and remove them through the Xray API so the
	// other clients keep their connections. Anything else needs a restart.
	if a.xray.running() && len(old) > 0 && a.hotUpdate(ctx, old, next) {
		return os.WriteFile(file, next, 0o600)
	}
	if err := os.WriteFile(file, next, 0o600); err != nil {
		return err
	}
	return a.xray.restart()
}

func (a *Agent) applyCore(p *proc, name string, next []byte, checkArgs []string) error {
	file := a.path(name)
	old, _ := os.ReadFile(file)
	if len(next) == 0 {
		p.stop()
		_ = os.Remove(file)
		return nil
	}
	if bytes.Equal(old, next) && p.running() {
		return nil
	}
	if err := a.check(p, next, checkArgs); err != nil {
		return err
	}
	if err := os.WriteFile(file, next, 0o600); err != nil {
		return err
	}
	return p.restart()
}

// check validates a config with the core binary before it replaces the old one.
func (a *Agent) check(p *proc, cfg []byte, args []string) error {
	tmp := a.path(p.name + ".check.json")
	if err := os.WriteFile(tmp, cfg, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.bin, append(args, tmp)...)
	cmd.Env = p.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config check failed: %s", coreError(out))
	}
	return nil
}

func (a *Agent) writeState(ok bool, errText string) {
	st := map[string]any{"ok": ok, "at": time.Now().UTC().Format(time.RFC3339), "error": errText, "version": a.version, "cores": a.cores}
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(a.path("state.json"), b, 0o600)
}

func coreVersions(cfg Config) string {
	var parts []string
	if v := firstLine(cfg.XrayBin, "version"); v != "" {
		f := strings.Fields(v)
		if len(f) >= 2 {
			parts = append(parts, "xray "+f[1])
		}
	}
	if v := firstLine(cfg.SingBoxBin, "version"); v != "" {
		f := strings.Fields(v)
		parts = append(parts, "sing-box "+f[len(f)-1])
	}
	return strings.Join(parts, ", ")
}

func firstLine(bin string, args ...string) string {
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// coreError picks the line that explains a failed config check: cores print
// a banner and info lines first, the reason last.
func coreError(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		low := strings.ToLower(l)
		if strings.Contains(low, "fail") || strings.Contains(low, "error") || strings.HasPrefix(low, "fatal") {
			// Xray: "Failed to start: main: failed to load config files: [path] > infra/conf: ..."
			if j := strings.Index(l, "] > "); j >= 0 {
				l = l[j+4:]
			}
			return l
		}
	}
	return lastLines(out, 3)
}

func lastLines(b []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// ports lists the public ports both cores listen on, for the firewall.
func ports(xray, singbox []byte) []int {
	set := map[int]bool{}
	var x struct {
		Inbounds []struct {
			Tag    string `json:"tag"`
			Listen string `json:"listen"`
			Port   int    `json:"port"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(xray, &x) == nil {
		for _, in := range x.Inbounds {
			if in.Port > 0 && in.Listen != "127.0.0.1" && in.Tag != "api" {
				set[in.Port] = true
			}
		}
	}
	var s struct {
		Inbounds []struct {
			ListenPort int `json:"listen_port"`
		} `json:"inbounds"`
		Endpoints []struct {
			ListenPort int `json:"listen_port"`
		} `json:"endpoints"`
	}
	if json.Unmarshal(singbox, &s) == nil {
		for _, in := range s.Inbounds {
			set[in.ListenPort] = in.ListenPort > 0
		}
		for _, in := range s.Endpoints {
			set[in.ListenPort] = in.ListenPort > 0
		}
	}
	var out []int
	for p, ok := range set {
		if ok {
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}
