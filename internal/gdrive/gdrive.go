// Package gdrive is a minimal Google Drive v3 client used to publish
// subscriptions as public files reachable through www.googleapis.com.
//
// Authorization uses the OAuth device flow ("TVs and Limited Input devices"
// client) with the drive.file scope, so the panel only ever sees files it
// created itself and works without a public HTTPS callback.
package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"
)

const Scope = "https://www.googleapis.com/auth/drive.file"

// Endpoints are overridable for tests.
type Endpoints struct {
	OAuth  string // https://oauth2.googleapis.com
	Drive  string // https://www.googleapis.com/drive/v3
	Upload string // https://www.googleapis.com/upload/drive/v3
}

var Google = Endpoints{
	OAuth:  "https://oauth2.googleapis.com",
	Drive:  "https://www.googleapis.com/drive/v3",
	Upload: "https://www.googleapis.com/upload/drive/v3",
}

type Client struct {
	ep   Endpoints
	http *http.Client

	mu           sync.Mutex
	clientID     string
	clientSecret string
	refreshToken string
	accessToken  string
	expires      time.Time
}

func New(ep Endpoints) *Client {
	return &Client{ep: ep, http: &http.Client{Timeout: 30 * time.Second}}
}

// Configure sets OAuth credentials and drops any cached access token.
func (c *Client) Configure(clientID, clientSecret, refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientID, c.clientSecret, c.refreshToken = clientID, clientSecret, refreshToken
	c.accessToken, c.expires = "", time.Time{}
}

// PublicURL is the link clients use to download a shared file with an API key.
func (c *Client) PublicURL(fileID, apiKey string) string {
	return c.ep.Drive + "/files/" + url.PathEscape(fileID) + "?key=" + url.QueryEscape(apiKey) + "&alt=media"
}

// APIError is a non-2xx response from Google.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := e.Body
	var parsed struct {
		Error any    `json:"error"`
		Desc  string `json:"error_description"`
	}
	if json.Unmarshal([]byte(e.Body), &parsed) == nil {
		switch v := parsed.Error.(type) {
		case string:
			msg = v
			if parsed.Desc != "" {
				msg += ": " + parsed.Desc
			}
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				msg = m
			}
		}
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Sprintf("google api %d: %s", e.Status, msg)
}

func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ep.OAuth+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

// ---- OAuth device flow ----

type DeviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func (c *Client) StartDeviceAuth(ctx context.Context, clientID string) (*DeviceAuth, error) {
	var d DeviceAuth
	err := c.postForm(ctx, "/device/code", url.Values{"client_id": {clientID}, "scope": {Scope}}, &d)
	if err != nil {
		return nil, err
	}
	if d.Interval <= 0 {
		d.Interval = 5
	}
	return &d, nil
}

var ErrPending = errors.New("authorization pending")

// PollDeviceAuth exchanges a device code for a refresh token. It returns
// ErrPending while the user has not approved access yet.
func (c *Client) PollDeviceAuth(ctx context.Context, clientID, clientSecret, deviceCode string) (string, error) {
	var tok struct {
		RefreshToken string `json:"refresh_token"`
	}
	err := c.postForm(ctx, "/token", url.Values{
		"client_id": {clientID}, "client_secret": {clientSecret}, "device_code": {deviceCode},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
	}, &tok)
	var apiErr *APIError
	if errors.As(err, &apiErr) && (strings.Contains(apiErr.Body, "authorization_pending") || strings.Contains(apiErr.Body, "slow_down")) {
		return "", ErrPending
	}
	if err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", errors.New("google returned no refresh token")
	}
	return tok.RefreshToken, nil
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshToken == "" {
		return "", errors.New("google drive is not connected")
	}
	if c.accessToken != "" && time.Now().Before(c.expires) {
		return c.accessToken, nil
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	err := c.postForm(ctx, "/token", url.Values{
		"client_id": {c.clientID}, "client_secret": {c.clientSecret},
		"refresh_token": {c.refreshToken}, "grant_type": {"refresh_token"},
	}, &tok)
	if err != nil {
		return "", err
	}
	c.accessToken = tok.AccessToken
	c.expires = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return c.accessToken, nil
}

func (c *Client) authed(ctx context.Context, method, url string, body io.Reader, contentType string, out any) error {
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.do(req, out)
}

// ---- Drive operations ----

// Account returns the email of the connected Google account (best effort).
func (c *Client) Account(ctx context.Context) (string, error) {
	var about struct {
		User struct {
			Email string `json:"emailAddress"`
		} `json:"user"`
	}
	err := c.authed(ctx, http.MethodGet, c.ep.Drive+"/about?fields=user", nil, "", &about)
	return about.User.Email, err
}

func (c *Client) CreateFolder(ctx context.Context, name string) (string, error) {
	meta, _ := json.Marshal(map[string]any{"name": name, "mimeType": "application/vnd.google-apps.folder"})
	var f struct {
		ID string `json:"id"`
	}
	err := c.authed(ctx, http.MethodPost, c.ep.Drive+"/files?fields=id", bytes.NewReader(meta), "application/json", &f)
	return f.ID, err
}

// CreateFile uploads a text file into folderID and returns its id.
func (c *Client) CreateFile(ctx context.Context, folderID, name string, content []byte) (string, error) {
	meta := map[string]any{"name": name, "mimeType": "text/plain"}
	if folderID != "" {
		meta["parents"] = []string{folderID}
	}
	metaJSON, _ := json.Marshal(meta)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", "application/json; charset=UTF-8")
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(metaJSON)
	h = textproto.MIMEHeader{}
	h.Set("Content-Type", "text/plain; charset=UTF-8")
	part, _ = mw.CreatePart(h)
	_, _ = part.Write(content)
	_ = mw.Close()

	var f struct {
		ID string `json:"id"`
	}
	err := c.authed(ctx, http.MethodPost, c.ep.Upload+"/files?uploadType=multipart&fields=id", &buf,
		"multipart/related; boundary="+mw.Boundary(), &f)
	return f.ID, err
}

func (c *Client) UpdateFile(ctx context.Context, fileID string, content []byte) error {
	return c.authed(ctx, http.MethodPatch, c.ep.Upload+"/files/"+url.PathEscape(fileID)+"?uploadType=media&fields=id",
		bytes.NewReader(content), "text/plain; charset=UTF-8", nil)
}

// SharePublic lets anyone with the file id read it.
func (c *Client) SharePublic(ctx context.Context, fileID string) error {
	body := strings.NewReader(`{"role":"reader","type":"anyone"}`)
	return c.authed(ctx, http.MethodPost, c.ep.Drive+"/files/"+url.PathEscape(fileID)+"/permissions?fields=id", body, "application/json", nil)
}

func (c *Client) Delete(ctx context.Context, fileID string) error {
	err := c.authed(ctx, http.MethodDelete, c.ep.Drive+"/files/"+url.PathEscape(fileID), nil, "", nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// FetchPublic downloads a file the way a client would: anonymously, with the API key.
func (c *Client) FetchPublic(ctx context.Context, fileID, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.PublicURL(fileID, apiKey), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}
