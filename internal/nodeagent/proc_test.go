package nodeagent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A core the system kills (the OOM killer: "signal: killed") is restarted,
// and its error goes away once it runs again for a while.
func TestCrashIsClearedOnceStable(t *testing.T) {
	old := stableAfter
	stableAfter = 300 * time.Millisecond
	defer func() { stableAfter = old }()

	dir := t.TempDir()
	script := filepath.Join(dir, "core.sh")
	marker := filepath.Join(dir, "ran")
	// first run: killed by a signal; later runs: keep going
	body := "#!/bin/sh\nif [ ! -f " + marker + " ]; then touch " + marker + "; echo about to die; kill -9 $$; fi\nexec sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	errText := ""
	set := func(m string) { mu.Lock(); errText = m; mu.Unlock() }
	get := func() string { mu.Lock(); defer mu.Unlock(); return errText }
	p := newProc("xray", script, nil, os.Environ(), slog.New(slog.DiscardHandler), set)
	p.onOK = func(m string) {
		mu.Lock()
		if errText == m {
			errText = ""
		}
		mu.Unlock()
	}
	if err := p.start(); err != nil {
		t.Fatal(err)
	}
	defer p.stop()

	deadline := time.Now().Add(3 * time.Second)
	for get() == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if e := get(); !strings.Contains(e, "signal: killed") {
		t.Fatalf("crash not reported: %q", e)
	}
	// restarted after 3s, then cleared after stableAfter
	deadline = time.Now().Add(6 * time.Second)
	for get() != "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if e := get(); e != "" || !p.running() {
		t.Fatalf("error kept after a stable restart: %q (running %v)", e, p.running())
	}
}
