package nodeagent

import (
	"bufio"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// proc supervises one core process: restarts it after a crash and reports
// the last output line as the node's error.
type proc struct {
	name  string
	bin   string
	args  []string
	env   []string
	log   *slog.Logger
	onErr func(string)
	onOK  func(string) // the core has run fine for a while after the given crash

	mu   sync.Mutex
	want bool
	cmd  *exec.Cmd
	done chan struct{}
	last string // last output line
	runs int    // starts so far; routing set through the API is lost on each
}

// stableAfter is how long a restarted core must run before its crash
// stops being reported.
var stableAfter = 2 * time.Minute

func newProc(name, bin string, args, env []string, log *slog.Logger, onErr func(string)) *proc {
	return &proc{name: name, bin: bin, args: args, env: env, log: log, onErr: onErr}
}

func (p *proc) running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil
}

func (p *proc) start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.want = true
	if p.cmd != nil {
		return nil
	}
	return p.spawnLocked()
}

func (p *proc) spawnLocked() error {
	cmd := exec.Command(p.bin, p.args...)
	cmd.Env = p.env
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	done := make(chan struct{})
	p.cmd, p.done = cmd, done
	p.runs++
	go p.pump(pr)
	go func() {
		err := cmd.Wait()
		pw.Close()
		close(done)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.cmd != cmd {
			return // stopped on purpose
		}
		p.cmd = nil
		if !p.want {
			return
		}
		msg := p.name + " exited"
		if err != nil {
			msg += " (" + err.Error() + ")"
		}
		if p.last != "" {
			msg += ": " + p.last
		}
		p.log.Warn("core crashed, restarting in 3s", "core", p.name, "err", msg)
		p.onErr(msg)
		time.AfterFunc(3*time.Second, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.want && p.cmd == nil {
				if err := p.spawnLocked(); err != nil {
					p.onErr(p.name + ": " + err.Error())
					return
				}
				// once it keeps running, the old crash is no longer news
				again := p.cmd
				time.AfterFunc(stableAfter, func() {
					p.mu.Lock()
					ok := p.cmd == again && p.onOK != nil
					p.mu.Unlock()
					if ok {
						p.onOK(msg)
					}
				})
			}
		})
	}()
	p.log.Info("core started", "core", p.name)
	return nil
}

func (p *proc) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p.mu.Lock()
		p.last = line
		p.mu.Unlock()
		p.log.Info(line, "core", p.name)
	}
}

func (p *proc) stop() {
	p.mu.Lock()
	p.want = false
	cmd, done := p.cmd, p.done
	p.cmd = nil
	p.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	p.log.Info("core stopped", "core", p.name)
}

func (p *proc) restart() error {
	p.stop()
	return p.start()
}

// generation changes every time the process is (re)started.
func (p *proc) generation() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runs
}
