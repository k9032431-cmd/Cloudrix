package nodeagent

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Low memory: when a small VPS runs out of RAM the kernel kills a process.
// The cores get a Go soft memory limit, so they collect garbage harder
// before the server is full, and a high OOM score, so the kernel picks a
// core (which the agent restarts in seconds) rather than the agent itself,
// which would cut the node off from the panel.

// coreOOMScore makes the cores the kernel's first choice when memory runs out.
const coreOOMScore = "500"

// memTotal is the server's RAM in bytes, 0 when unknown.
func memTotal() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if kb, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(kb), "kB")), 10, 64)
			if err == nil {
				return n * 1024
			}
		}
	}
	return 0
}

// coreEnv adds a soft memory limit for the cores (70% of RAM) unless one
// is set already.
func coreEnv(env []string, total int64) []string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOMEMLIMIT=") {
			return env
		}
	}
	if total <= 0 {
		return env
	}
	return append(env, fmt.Sprintf("GOMEMLIMIT=%dMiB", total*7/10>>20))
}

// raiseOOMScore marks a started core as the first to go when memory runs
// out. It only works on Linux and as root; failures are ignored.
func raiseOOMScore(pid int) {
	_ = os.WriteFile(fmt.Sprintf("/proc/%d/oom_score_adj", pid), []byte(coreOOMScore), 0o644)
}
