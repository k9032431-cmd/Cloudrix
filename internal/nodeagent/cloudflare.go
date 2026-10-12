package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CloudflareAPI is the API base; tests point it at a fake server.
var CloudflareAPI = "https://api.cloudflare.com/client/v4"

type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func cfCall(ctx context.Context, token, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, CloudflareAPI+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var r cfResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r); err != nil {
		return fmt.Errorf("cloudflare: HTTP %d", res.StatusCode)
	}
	if !r.Success {
		var msgs []string
		for _, e := range r.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("cloudflare: %s", strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// CloudflareZone finds the id of the Cloudflare zone holding domain: the
// longest suffix of the domain the token can see.
func CloudflareZone(ctx context.Context, token, domain string) (string, error) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	labels := strings.Split(domain, ".")
	for i := 0; i < len(labels)-1; i++ {
		name := strings.Join(labels[i:], ".")
		var zones []struct {
			ID string `json:"id"`
		}
		if err := cfCall(ctx, token, "GET", "/zones?name="+url.QueryEscape(name), nil, &zones); err != nil {
			return "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("no Cloudflare zone for %s visible to this token: add the domain to Cloudflare and give the token Zone → DNS → Edit", domain)
}

// CloudflarePointA makes domain resolve to ip with an A record that is not
// proxied (Hysteria2 runs over UDP, which the Cloudflare proxy does not
// carry). The token needs Zone → DNS → Edit on the domain's zone.
// It returns the zone id, which acme.sh needs for the DNS challenge.
func CloudflarePointA(ctx context.Context, token, domain, ip string) (string, error) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if token == "" || domain == "" || ip == "" {
		return "", errors.New("domain, IP and Cloudflare token are required")
	}
	zoneID, err := CloudflareZone(ctx, token, domain)
	if err != nil {
		return "", err
	}
	var records []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		Proxied bool   `json:"proxied"`
	}
	q := "/zones/" + zoneID + "/dns_records?type=A&name=" + url.QueryEscape(domain)
	if err := cfCall(ctx, token, "GET", q, nil, &records); err != nil {
		return "", err
	}
	rec := map[string]any{"type": "A", "name": domain, "content": ip, "ttl": 60, "proxied": false}
	if len(records) == 0 {
		return zoneID, cfCall(ctx, token, "POST", "/zones/"+zoneID+"/dns_records", rec, nil)
	}
	if records[0].Content == ip && !records[0].Proxied {
		return zoneID, nil
	}
	return zoneID, cfCall(ctx, token, "PUT", "/zones/"+zoneID+"/dns_records/"+records[0].ID, rec, nil)
}
