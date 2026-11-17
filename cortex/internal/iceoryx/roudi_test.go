package iceoryx

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain doubles as a fake RouDi: when FAKE_ROUDI is set, the test binary
// listens on $IOX_DIR/roudi until SIGTERM (or exits immediately if
// FAKE_ROUDI=fail).
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_ROUDI") {
	case "":
		os.Exit(m.Run())
	case "fail":
		os.Exit(3)
	default:
		l, err := net.Listen("unix", filepath.Join(os.Getenv("IOX_DIR"), "roudi"))
		if err != nil {
			os.Exit(2)
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM)
		<-sig
		l.Close()
		os.Exit(0)
	}
}

type nopLog struct{}

func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

type fakeStore struct {
	mu     sync.Mutex
	states map[string]string
}

func (f *fakeStore) set(id, st string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[id] = st
}

func (f *fakeStore) get(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.states[id]
	return st, ok
}

func newTestSupervisor(t *testing.T, mode string, onCrash CrashFunc) (*Supervisor, *fakeStore) {
	t.Helper()
	t.Setenv("FAKE_ROUDI", mode)
	// Unix socket paths are limited to ~108 bytes; t.TempDir can be long.
	dir, err := os.MkdirTemp("", "iox")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{states: map[string]string{}}
	s := New(Config{
		LaunchCmd:    []string{exe},
		SocketDir:    dir,
		LogPath:      filepath.Join(dir, "roudi.log"),
		ReadyTimeout: 5 * time.Second,
		Grace:        time.Minute,
		StopTimeout:  2 * time.Second,
	}, nopLog{}, fs.get, onCrash)
	t.Cleanup(func() {
		s.mu.Lock()
		p := s.proc
		s.mu.Unlock()
		if p != nil {
			s.terminate(p)
		}
	})
	return s, fs
}

func TestAcquireStartsOnceAndIdleStops(t *testing.T) {
	s, fs := newTestSupervisor(t, "ok", nil)
	ctx := context.Background()

	fs.set("a", "starting")
	fs.set("b", "running")
	if err := s.Acquire(ctx, "a"); err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	s.mu.Lock()
	first := s.proc
	s.mu.Unlock()
	if err := s.Acquire(ctx, "b"); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	s.mu.Lock()
	second := s.proc
	s.mu.Unlock()
	if first != second {
		t.Fatal("RouDi was started twice")
	}
	if !s.Running() || s.Users() != 2 {
		t.Fatalf("running=%v users=%d", s.Running(), s.Users())
	}

	now := time.Now()
	s.tick(now)
	if s.Users() != 2 {
		t.Fatalf("users pruned while active: %d", s.Users())
	}

	// Both instances stop → pruned; RouDi survives the grace period...
	fs.set("a", "stopped")
	fs.set("b", "crashed")
	s.tick(now)
	if s.Users() != 0 || !s.Running() {
		t.Fatalf("after prune: users=%d running=%v", s.Users(), s.Running())
	}
	s.tick(now.Add(30 * time.Second))
	if !s.Running() {
		t.Fatal("RouDi stopped before grace period elapsed")
	}

	// ...and is stopped after it.
	s.tick(now.Add(61 * time.Second))
	if s.Running() {
		t.Fatal("RouDi still running after grace period")
	}
	if !first.exited() {
		t.Fatal("RouDi process not terminated")
	}
}

func TestReacquireDuringGraceCancelsIdle(t *testing.T) {
	s, fs := newTestSupervisor(t, "ok", nil)
	fs.set("a", "running")
	if err := s.Acquire(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fs.set("a", "stopped")
	s.tick(now) // idle starts

	fs.set("b", "starting")
	if err := s.Acquire(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	s.tick(now.Add(2 * time.Minute))
	if !s.Running() {
		t.Fatal("RouDi stopped although a user re-acquired during grace")
	}
}

func TestAcquireFailsWhenRoudiExits(t *testing.T) {
	s, fs := newTestSupervisor(t, "fail", nil)
	fs.set("a", "starting")
	if err := s.Acquire(context.Background(), "a"); err == nil {
		t.Fatal("expected error when RouDi exits during startup")
	}
	if s.Running() || s.Users() != 0 {
		t.Fatalf("running=%v users=%d", s.Running(), s.Users())
	}
}

func TestAcquireFailsWithoutLauncher(t *testing.T) {
	s, _ := newTestSupervisor(t, "ok", nil)
	s.cfg.LaunchCmd = []string{"/nonexistent/zaraos-roudi.sh"}
	if err := s.Acquire(context.Background(), "a"); err == nil {
		t.Fatal("expected error for missing launcher")
	}
}

func TestCrashNotifiesUsers(t *testing.T) {
	var got []string
	s, fs := newTestSupervisor(t, "ok", func(ids []string, err error) { got = ids })
	fs.set("a", "running")
	if err := s.Acquire(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	_ = p.cmd.Process.Kill()
	<-p.done

	s.tick(time.Now())
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("onCrash got %v, want [a]", got)
	}
	if s.Running() || s.Users() != 0 {
		t.Fatalf("running=%v users=%d", s.Running(), s.Users())
	}

	// A new Acquire starts a fresh RouDi (stale socket cleared first).
	if err := s.Acquire(context.Background(), "a"); err != nil {
		t.Fatalf("re-acquire after crash: %v", err)
	}
}
