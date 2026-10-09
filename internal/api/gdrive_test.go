package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/gdrive"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// fakeGoogle emulates the parts of OAuth and Drive v3 the panel uses.
type fakeGoogle struct {
	mu        sync.Mutex
	polls     int
	files     map[string]string // id -> content
	public    map[string]bool
	folders   map[string]bool
	nextID    int
	apiKey    string
	refreshes int
}

func newFakeGoogle() (*fakeGoogle, *httptest.Server) {
	g := &fakeGoogle{files: map[string]string{}, public: map[string]bool{}, folders: map[string]bool{}, apiKey: "AIzaTEST"}
	return g, httptest.NewServer(g)
}

func (g *fakeGoogle) id() string {
	g.nextID++
	return "f" + string(rune('A'+g.nextID))
}

func (g *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := r.URL.Path
	authed := r.Header.Get("Authorization") == "Bearer access-1"
	jsonOut := func(v any) { _ = json.NewEncoder(w).Encode(v) }

	switch {
	case p == "/oauth/device/code":
		jsonOut(map[string]any{"device_code": "dev-1", "user_code": "ABCD-EFGH", "verification_url": "https://www.google.com/device", "expires_in": 60, "interval": 1})
	case p == "/oauth/token":
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			g.polls++
			if g.polls < 2 {
				w.WriteHeader(428)
				jsonOut(map[string]string{"error": "authorization_pending"})
				return
			}
			jsonOut(map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600})
		case "refresh_token":
			g.refreshes++
			if r.Form.Get("refresh_token") != "refresh-1" || r.Form.Get("client_secret") != "secret" {
				w.WriteHeader(400)
				jsonOut(map[string]string{"error": "invalid_grant"})
				return
			}
			jsonOut(map[string]any{"access_token": "access-1", "expires_in": 3600})
		}
	case p == "/drive/v3/about":
		jsonOut(map[string]any{"user": map[string]string{"emailAddress": "owner@gmail.com"}})
	case p == "/drive/v3/files" && r.Method == http.MethodPost && authed:
		id := g.id()
		g.folders[id] = true
		jsonOut(map[string]string{"id": id})
	case p == "/upload/drive/v3/files" && r.Method == http.MethodPost && authed:
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := multipart.NewReader(r.Body, params["boundary"])
		metaPart, _ := mr.NextPart()
		var meta struct {
			Parents []string `json:"parents"`
		}
		_ = json.NewDecoder(metaPart).Decode(&meta)
		if len(meta.Parents) != 1 || !g.folders[meta.Parents[0]] {
			w.WriteHeader(404)
			jsonOut(map[string]any{"error": map[string]string{"message": "File not found: parent"}})
			return
		}
		body, _ := mr.NextPart()
		content, _ := io.ReadAll(body)
		id := g.id()
		g.files[id] = string(content)
		jsonOut(map[string]string{"id": id})
	case strings.HasPrefix(p, "/upload/drive/v3/files/") && r.Method == http.MethodPatch && authed:
		id := strings.TrimPrefix(p, "/upload/drive/v3/files/")
		if _, ok := g.files[id]; !ok {
			w.WriteHeader(404)
			return
		}
		b, _ := io.ReadAll(r.Body)
		g.files[id] = string(b)
		jsonOut(map[string]string{"id": id})
	case strings.HasSuffix(p, "/permissions") && authed:
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/drive/v3/files/"), "/permissions")
		g.public[id] = true
		jsonOut(map[string]string{"id": "perm"})
	case strings.HasPrefix(p, "/drive/v3/files/") && r.Method == http.MethodDelete && authed:
		id := strings.TrimPrefix(p, "/drive/v3/files/")
		delete(g.files, id)
		delete(g.folders, id)
		w.WriteHeader(204)
	case strings.HasPrefix(p, "/drive/v3/files/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(p, "/drive/v3/files/")
		content, ok := g.files[id]
		if r.URL.Query().Get("key") != g.apiKey || r.URL.Query().Get("alt") != "media" {
			w.WriteHeader(400)
			return
		}
		if !ok || !g.public[id] {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, content)
	default:
		w.WriteHeader(401)
	}
}

func (g *fakeGoogle) content(id string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.files[id]
	return c, ok
}

func TestGoogleDriveSubscriptions(t *testing.T) {
	e := newEnv(t)
	g, gs := newFakeGoogle()
	defer gs.Close()
	// Point the running server's syncer at the fake Google.
	e.api.drive.client = gdrive.New(gdrive.Endpoints{OAuth: gs.URL + "/oauth", Drive: gs.URL + "/drive/v3", Upload: gs.URL + "/upload/drive/v3"})

	root := e.login("root", "supersecret")
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, Enabled: true, Remark: "{USERNAME}", Settings: model.InboundSettings{Security: "tls"}}, nil)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, &u)

	// Not configured yet: the user's link is unavailable.
	if resp := e.do("POST", "/api/users/"+itoa(u.ID)+"/gdrive", root, nil, nil); resp.StatusCode != 400 {
		t.Fatalf("drive before setup: %d", resp.StatusCode)
	}

	// Configure and connect through the device flow.
	e.do("PUT", "/api/settings/gdrive", root, map[string]any{"enabled": true, "api_key": g.apiKey, "client_id": "cid", "client_secret": "secret", "format": "base64", "interval_minutes": 10}, nil)
	var conn driveConnect
	if resp := e.do("POST", "/api/settings/gdrive/connect", root, nil, &conn); resp.StatusCode != 200 || conn.UserCode != "ABCD-EFGH" {
		t.Fatalf("connect: %d %+v", resp.StatusCode, conn)
	}
	var view driveView
	for i := 0; i < 50; i++ {
		e.do("GET", "/api/settings/gdrive", root, nil, &view)
		if view.Connected {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !view.Connected || view.Account != "owner@gmail.com" || view.HasClientSecret != true {
		t.Fatalf("not connected: %+v", view)
	}

	if resp := e.do("POST", "/api/settings/gdrive/test", root, nil, nil); resp.StatusCode != 200 {
		t.Fatalf("self test: %d", resp.StatusCode)
	}

	// Create the user's link and fetch it like a client would.
	var d userDriveView
	if resp := e.do("POST", "/api/users/"+itoa(u.ID)+"/gdrive", root, nil, &d); resp.StatusCode != 200 {
		t.Fatalf("create link: %d", resp.StatusCode)
	}
	wantPrefix := gs.URL + "/drive/v3/files/"
	if !strings.HasPrefix(d.URL, wantPrefix) || !strings.HasSuffix(d.URL, "?key=AIzaTEST&alt=media") {
		t.Fatalf("link = %s", d.URL)
	}
	fetch := func(url string) (int, string) {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := fetch(d.URL)
	links, _ := base64.StdEncoding.DecodeString(body)
	if code != 200 || !strings.HasPrefix(string(links), "trojan://") {
		t.Fatalf("drive content: %d %q", code, links)
	}
	fileID := strings.TrimSuffix(strings.TrimPrefix(d.URL, wantPrefix), "?key=AIzaTEST&alt=media")

	// The public subscription info exposes the backup link.
	var info subscriptionInfo
	e.do("GET", "/sub/"+u.SubToken+"/info", "", nil, &info)
	if info.DriveURL != d.URL {
		t.Fatalf("info gdrive_url = %q", info.DriveURL)
	}

	// Disabling the user empties the Drive copy after a sync.
	e.do("PUT", "/api/users/"+itoa(u.ID), root, userInput{Username: "alice", Status: model.StatusDisabled}, nil)
	e.do("POST", "/api/settings/gdrive/sync", root, nil, nil)
	if c, _ := g.content(fileID); c != "" {
		t.Fatalf("disabled user still has content: %q", c)
	}

	// Plain format carries subscription metadata as comments.
	e.do("PUT", "/api/users/"+itoa(u.ID), root, userInput{Username: "alice", DataLimit: 1 << 30}, nil)
	e.do("PUT", "/api/settings/gdrive", root, map[string]any{"enabled": true, "api_key": g.apiKey, "client_id": "cid", "format": "plain", "interval_minutes": 10}, nil)
	e.do("POST", "/api/settings/gdrive/sync", root, nil, nil)
	if c, _ := g.content(fileID); !strings.Contains(c, "#subscription-userinfo: upload=0; download=0; total=1073741824") || !strings.Contains(c, "\ntrojan://") {
		t.Fatalf("plain content: %q", c)
	}

	// Rotating keys replaces the Drive file: the old link dies.
	var revoked model.User
	e.do("POST", "/api/users/"+itoa(u.ID)+"/revoke", root, nil, &revoked)
	if code, _ := fetch(d.URL); code != 404 {
		t.Fatalf("old drive link still works: %d", code)
	}
	var sub struct {
		GDrive userDriveView `json:"gdrive"`
	}
	e.do("GET", "/api/users/"+itoa(u.ID)+"/subscription", root, nil, &sub)
	if sub.GDrive.URL == "" || sub.GDrive.URL == d.URL {
		t.Fatalf("no new drive link after revoke: %+v", sub.GDrive)
	}
	if code, body := fetch(sub.GDrive.URL); code != 200 || !strings.Contains(body, revoked.Credentials.Password) {
		t.Fatalf("new link content: %d %q", code, body)
	}

	// A device limit cannot be enforced through Drive: the link is refused.
	e.do("PUT", "/api/users/"+itoa(u.ID), root, userInput{Username: "alice", DeviceLimit: 2}, nil)
	if resp := e.do("POST", "/api/users/"+itoa(u.ID)+"/gdrive", root, nil, nil); resp.StatusCode != 400 {
		t.Fatalf("device-limited user got a drive link: %d", resp.StatusCode)
	}
	if code, _ := fetch(sub.GDrive.URL); code != 404 {
		t.Fatalf("device-limited user's drive file must be removed: %d", code)
	}

	// Deleting a user removes their Drive file.
	var bob model.User
	e.do("POST", "/api/users", root, userInput{Username: "bob"}, &bob)
	var bd userDriveView
	e.do("POST", "/api/users/"+itoa(bob.ID)+"/gdrive", root, nil, &bd)
	e.do("DELETE", "/api/users/"+itoa(bob.ID), root, nil, nil)
	for i := 0; i < 50; i++ {
		if code, _ := fetch(bd.URL); code == 404 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("deleted user's drive file still served")
}
