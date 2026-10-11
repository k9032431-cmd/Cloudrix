package nodeagent

import (
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// firewall opens inbound ports when ufw or firewalld is active, so a new
// inbound works without logging in to the node.
type firewall struct {
	log    *slog.Logger
	opened map[int]bool
}

func newFirewall(log *slog.Logger) *firewall { return &firewall{log: log, opened: map[int]bool{}} }

func (f *firewall) allow(ports []int) {
	var todo []int
	for _, p := range ports {
		if !f.opened[p] {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return
	}
	switch {
	case ufwActive():
		for _, p := range todo {
			if exec.Command("ufw", "allow", strconv.Itoa(p)).Run() == nil {
				f.log.Info("opened port in ufw", "port", p)
			}
		}
	case firewalldActive():
		for _, p := range todo {
			for _, proto := range []string{"tcp", "udp"} {
				_ = exec.Command("firewall-cmd", "--permanent", "--add-port="+strconv.Itoa(p)+"/"+proto).Run()
			}
		}
		if exec.Command("firewall-cmd", "--reload").Run() == nil {
			f.log.Info("opened ports in firewalld", "ports", todo)
		}
	}
	for _, p := range todo {
		f.opened[p] = true
	}
}

func ufwActive() bool {
	out, err := exec.Command("ufw", "status").Output()
	return err == nil && strings.Contains(string(out), "Status: active")
}

func firewalldActive() bool {
	return exec.Command("firewall-cmd", "--state").Run() == nil
}
