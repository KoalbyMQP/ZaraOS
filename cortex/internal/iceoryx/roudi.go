// Package iceoryx supervises the iceoryx RouDi daemon that CycloneDDS needs
// for its shared-memory transport.
//
// RouDi is started lazily, the first time a shared_memory container is about
// to start (Acquire). It is stopped again once no such container has been
// running for a grace period, so robots that don't use shared memory pay
// nothing for it (RouDi preallocates its whole mempool in /dev/shm).
//
// Users are tracked by instance ID; the supervisor polls the instance state
// (via StateFunc) and drops users whose instance is no longer starting or
// running, so callers never need to "release" explicitly.
//
// When this package is no longer needed (e.g. after migrating to Fast DDS,
// which uses /dev/shm without a broker) it can be deleted together with the
// SharedMemory option.
package iceoryx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Defaults match the ZaraOS rootfs (overlays/usr/bin/zaraos-roudi.sh).
const (
	DefaultLaunchCmd = "/usr/bin/zaraos-roudi.sh"
	DefaultSocketDir = "/tmp/iceoryx" // == requirements.IceoryxSocketDir
	DefaultLogPath   = "/var/log/roudi.log"
	roudiSocketName  = "roudi" // iceoryx IPC_CHANNEL_ROUDI_NAME
	roudiProcName    = "iox-roudi"
)

// Logger is the subset of cortex/internal/logger used here.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// StateFunc returns the state of an instance ("starting", "running", ...),
// or ok=false if it no longer exists.
type StateFunc func(instanceID string) (state string, ok bool)

// CrashFunc is called (without locks held) when RouDi exits unexpectedly
// while instances still depend on it. iceoryx clients cannot recover from
// losing RouDi, so the callback should stop/restart those instances.
type CrashFunc func(instanceIDs []string, err error)

// Config tunes the supervisor. Zero values fall back to defaults.
type Config struct {
	LaunchCmd    []string      // command that execs RouDi in place
	SocketDir    string        // host dir RouDi sees as /tmp
	LogPath      string        // RouDi stdout/stderr ("" → DefaultLogPath)
	ReadyTimeout time.Duration // max wait for RouDi's socket
	Grace        time.Duration // idle time before stopping RouDi
	PollInterval time.Duration // user pruning / crash detection period
	StopTimeout  time.Duration // SIGTERM → SIGKILL escalation
}

func (c *Config) setDefaults() {
	if len(c.LaunchCmd) == 0 {
		c.LaunchCmd = []string{DefaultLaunchCmd}
	}
	if c.SocketDir == "" {
		c.SocketDir = DefaultSocketDir
	}
	if c.LogPath == "" {
		c.LogPath = DefaultLogPath
	}
	if c.ReadyTimeout == 0 {
		c.ReadyTimeout = 10 * time.Second
	}
	if c.Grace == 0 {
		c.Grace = 30 * time.Second
	}
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.StopTimeout == 0 {
		c.StopTimeout = 5 * time.Second
	}
}

// proc is one running RouDi process.
type proc struct {
	cmd  *exec.Cmd
	done chan struct{} // closed when the process exits
	err  error         // exit error, valid after done is closed
}

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Supervisor starts and stops RouDi on demand.
type Supervisor struct {
	cfg     Config
	log     Logger
	state   StateFunc
	onCrash CrashFunc

	startMu sync.Mutex // serializes Acquire so only one start is in flight

	mu        sync.Mutex
	users     map[string]struct{}
	proc      *proc // nil when RouDi is not (intentionally) running
	idleSince time.Time
}

// New creates a supervisor. onCrash may be nil.
func New(cfg Config, log Logger, state StateFunc, onCrash CrashFunc) *Supervisor {
	cfg.setDefaults()
	return &Supervisor{
		cfg:     cfg,
		log:     log,
		state:   state,
		onCrash: onCrash,
		users:   make(map[string]struct{}),
	}
}

// Acquire registers instanceID as a RouDi user and makes sure RouDi is
// running and accepting connections before returning.
func (s *Supervisor) Acquire(ctx context.Context, instanceID string) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	s.mu.Lock()
	if s.proc != nil && !s.proc.exited() {
		s.users[instanceID] = struct{}{}
		s.idleSince = time.Time{}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	p, err := s.start(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.proc = p
	s.users[instanceID] = struct{}{}
	s.idleSince = time.Time{}
	s.mu.Unlock()
	return nil
}

// Running reports whether RouDi is currently running.
func (s *Supervisor) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc != nil && !s.proc.exited()
}

// Users returns the number of instances currently depending on RouDi.
func (s *Supervisor) Users() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

// start launches RouDi and waits for its socket. Caller holds startMu.
func (s *Supervisor) start(ctx context.Context) (*proc, error) {
	if _, err := exec.LookPath(s.cfg.LaunchCmd[0]); err != nil {
		return nil, fmt.Errorf("roudi launcher not found: %w", err)
	}

	// Fresh state: no RouDi is running (orphans were killed by Cleanup), so
	// any socket left behind is stale and would fake readiness.
	if err := os.MkdirAll(s.cfg.SocketDir, 0o777); err != nil {
		return nil, fmt.Errorf("create %s: %w", s.cfg.SocketDir, err)
	}
	_ = os.Chmod(s.cfg.SocketDir, 0o777|os.ModeSticky) // like /tmp: any app UID
	clearDir(s.cfg.SocketDir)

	cmd := exec.Command(s.cfg.LaunchCmd[0], s.cfg.LaunchCmd[1:]...)
	cmd.Env = append(os.Environ(), "IOX_DIR="+s.cfg.SocketDir)
	// Own process group: signals aimed at cortex don't reach RouDi.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if f, err := os.OpenFile(s.cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close() // the child keeps its own descriptor
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start roudi: %w", err)
	}
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()

	s.log.Info("iceoryx: starting RouDi", "pid", cmd.Process.Pid)

	sock := filepath.Join(s.cfg.SocketDir, roudiSocketName)
	deadline := time.NewTimer(s.cfg.ReadyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if fi, err := os.Stat(sock); err == nil && fi.Mode()&os.ModeSocket != 0 {
			s.log.Info("iceoryx: RouDi ready", "pid", cmd.Process.Pid, "socket", sock)
			return p, nil
		}
		select {
		case <-p.done:
			return nil, fmt.Errorf("roudi exited during startup: %v (see %s)", p.err, s.cfg.LogPath)
		case <-ctx.Done():
			s.terminate(p)
			return nil, ctx.Err()
		case <-deadline.C:
			s.terminate(p)
			return nil, fmt.Errorf("roudi socket %s not ready after %s (see %s)", sock, s.cfg.ReadyTimeout, s.cfg.LogPath)
		case <-tick.C:
		}
	}
}

// terminate stops a RouDi process: SIGTERM, then SIGKILL after StopTimeout.
func (s *Supervisor) terminate(p *proc) {
	if p.exited() {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(s.cfg.StopTimeout):
		s.log.Warn("iceoryx: RouDi did not exit on SIGTERM, killing", "pid", p.cmd.Process.Pid)
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// Run drives user pruning, idle shutdown and crash detection until ctx is
// cancelled. RouDi is left running on return: containers outlive cortex,
// and killing RouDi would break them.
func (s *Supervisor) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(time.Now())
		}
	}
}

// tick performs one supervision step. Exposed to tests via the package.
func (s *Supervisor) tick(now time.Time) {
	// Hold startMu so an in-flight Acquire can't race the idle shutdown.
	s.startMu.Lock()
	defer s.startMu.Unlock()

	s.mu.Lock()
	for id := range s.users {
		if st, ok := s.state(id); !ok || (st != "starting" && st != "running") {
			delete(s.users, id)
		}
	}

	p := s.proc
	if p == nil {
		s.mu.Unlock()
		return
	}

	// Unexpected exit.
	if p.exited() {
		s.proc = nil
		var affected []string
		for id := range s.users {
			affected = append(affected, id)
		}
		s.users = make(map[string]struct{})
		s.mu.Unlock()

		if len(affected) > 0 {
			s.log.Error("iceoryx: RouDi exited unexpectedly", "err", p.err, "users", len(affected))
			if s.onCrash != nil {
				s.onCrash(affected, fmt.Errorf("iceoryx RouDi exited: %v", p.err))
			}
		} else {
			s.log.Warn("iceoryx: RouDi exited while idle", "err", p.err)
		}
		return
	}

	if len(s.users) > 0 {
		s.idleSince = time.Time{}
		s.mu.Unlock()
		return
	}
	if s.idleSince.IsZero() {
		s.idleSince = now
		s.mu.Unlock()
		return
	}
	if now.Sub(s.idleSince) < s.cfg.Grace {
		s.mu.Unlock()
		return
	}

	// Idle past the grace period: stop RouDi.
	s.proc = nil
	s.idleSince = time.Time{}
	s.mu.Unlock()

	s.log.Info("iceoryx: no shared-memory apps running, stopping RouDi", "pid", p.cmd.Process.Pid)
	s.terminate(p)
}

// Cleanup kills RouDi processes left over from a previous cortex run and
// clears stale sockets. Call once at startup, before any Acquire.
func (s *Supervisor) Cleanup() {
	for _, pid := range findProcs(roudiProcName) {
		s.log.Warn("iceoryx: killing orphaned RouDi", "pid", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	clearDir(s.cfg.SocketDir)
}

// findProcs returns the PIDs whose /proc/<pid>/comm equals name.
func findProcs(name string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		var pid int
		if _, err := fmt.Sscanf(e.Name(), "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err == nil && strings.TrimSpace(string(comm)) == name {
			pids = append(pids, pid)
		}
	}
	return pids
}

// clearDir removes the contents of dir (not dir itself).
func clearDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}
