package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflarePointA(t *testing.T) {
	type rec struct {
		ID, Name, Content string
		Proxied           bool
	}
	records := map[string]*rec{}
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tok" {
			json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]string{{"message": "Invalid API Token"}}})
			return
		}
		ok := func(v any) { json.NewEncoder(w).Encode(map[string]any{"success": true, "result": v}) }
		switch {
		case r.URL.Path == "/zones":
			if r.URL.Query().Get("name") == "example.com" {
				ok([]map[string]string{{"id": "z1", "name": "example.com"}})
			} else {
				ok([]any{})
			}
		case r.URL.Path == "/zones/z1/dns_records" && r.Method == "GET":
			var out []map[string]any
			for _, x := range records {
				if x.Name == r.URL.Query().Get("name") {
					out = append(out, map[string]any{"id": x.ID, "content": x.Content, "proxied": x.Proxied})
				}
			}
			ok(out)
		case strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records"):
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/zones/z1/dns_records"), "/")
			if id == "" {
				id = "r1"
			}
			records[id] = &rec{ID: id, Name: body["name"].(string), Content: body["content"].(string), Proxied: body["proxied"].(bool)}
			ok(map[string]string{"id": id})
		}
	}))
	defer srv.Close()
	CloudflareAPI = srv.URL
	ctx := context.Background()

	if _, err := CloudflarePointA(ctx, "bad", "hy.example.com", "1.2.3.4"); err == nil || !strings.Contains(err.Error(), "Invalid API Token") {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := CloudflarePointA(ctx, "tok", "hy.other.net", "1.2.3.4"); err == nil || !strings.Contains(err.Error(), "no Cloudflare zone") {
		t.Fatalf("unknown zone: %v", err)
	}
	if zone, err := CloudflarePointA(ctx, "tok", "HY.example.com.", "1.2.3.4"); err != nil || zone != "z1" {
		t.Fatal(zone, err)
	}
	if r := records["r1"]; r == nil || r.Name != "hy.example.com" || r.Content != "1.2.3.4" || r.Proxied {
		t.Fatalf("created record: %+v", r)
	}
	// a new IP updates the same record instead of adding a second one
	if _, err := CloudflarePointA(ctx, "tok", "hy.example.com", "5.6.7.8"); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records["r1"].Content != "5.6.7.8" {
		t.Fatalf("updated records: %+v", records)
	}
	if last := calls[len(calls)-1]; last != "PUT /zones/z1/dns_records/r1" {
		t.Fatalf("last call %s", last)
	}
}
