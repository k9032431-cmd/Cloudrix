package happ

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCrypt4(t *testing.T) {
	l, err := Crypt4("https://vpn.example.com/sub/abcdef")
	if err != nil || !strings.HasPrefix(l, "happ://crypt4/") || len(l) < 600 {
		t.Fatalf("crypt4: %q %v", l, err)
	}
}

func TestEncryptorUsesServiceAndFallsBack(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"encrypted_link":"happ://crypt5/abc"}`))
	}))
	defer srv.Close()
	APIURL = srv.URL
	e := NewEncryptor()
	for i := 0; i < 2; i++ {
		if l, err := e.Link(context.Background(), "https://x/sub/1"); err != nil || l != "happ://crypt5/abc" {
			t.Fatalf("link: %q %v", l, err)
		}
	}
	if calls != 1 {
		t.Fatalf("service called %d times, want 1 (cached)", calls)
	}
	srv.Close()
	if l, err := e.Link(context.Background(), "https://x/sub/2"); err != nil || !strings.HasPrefix(l, "happ://crypt4/") {
		t.Fatalf("fallback: %q %v", l, err)
	}
}
