package store

import (
	"fmt"
	"sync"
	"time"
)

// --- App ---

type App struct {
	Name          string
	Repo          string
	Description   string
	LatestVersion string
}

type AppVersion struct {
	Version     string
	PublishedAt time.Time
	Changelog   string
}

// --- Instance ---

type Instance struct {
	ID        string
	App       string
	Version   string
	State     string // starting | running | stopping | stopped | crashed
	StartedAt time.Time
	StoppedAt *time.Time
	PID       *int
	ExitCode  *int
}

// --- Event ---

type Event struct {
	ID        string
	Type      string
	Timestamp time.Time
	Data      map[string]any
}

// --- Diagnostics ---

type DiagTest struct {
	Name           string
	PassedOutput   string
	FailedOutput   string
	DurationMs     int
	DefaultPassing bool
}

type DiagResult struct {
	Name       string
	Passed     bool
	Output     string
	DurationMs int
	RanAt      time.Time
}

// --- Identity ---

type Identity struct {
	Name            string
	Serial          string
	Location        string
	FirmwareVersion string
}

// --- Store ---

type Store struct {
	mu sync.RWMutex

	apps        []App
	versions    map[string][]AppVersion
	instances   map[string]*Instance
	events      []Event
	eventSeq    int
	diagTests   []DiagTest
	diagResults map[string]*DiagResult
	identity    Identity
}

func New() *Store {
	s := &Store{
		versions:    make(map[string][]AppVersion),
		instances:   make(map[string]*Instance),
		diagResults: make(map[string]*DiagResult),
	}
	s.seed()
	return s
}

func (s *Store) seed() {
	s.apps = []App{
		{Name: "ros2-nav", Repo: "myorg/ros2-nav", Description: "Navigation stack", LatestVersion: "1.5.0"},
		{Name: "camera-driver", Repo: "myorg/camera-driver", Description: "Camera capture and streaming", LatestVersion: "2.1.0"},
		{Name: "lidar-proc", Repo: "myorg/lidar-proc", Description: "LIDAR point cloud processor", LatestVersion: "0.9.3"},
	}

	s.versions["ros2-nav"] = []AppVersion{
		{Version: "1.5.0", PublishedAt: time.Now().Add(-24 * time.Hour), Changelog: "Improved path planning algorithm"},
		{Version: "1.4.0", PublishedAt: time.Now().Add(-72 * time.Hour), Changelog: "Added obstacle avoidance improvements"},
		{Version: "1.3.2", PublishedAt: time.Now().Add(-200 * time.Hour), Changelog: "Bugfix: LIDAR timeout handling"},
	}
	s.versions["camera-driver"] = []AppVersion{
		{Version: "2.1.0", PublishedAt: time.Now().Add(-48 * time.Hour), Changelog: "H264 streaming support"},
		{Version: "2.0.1", PublishedAt: time.Now().Add(-120 * time.Hour), Changelog: "Fixed memory leak in capture loop"},
	}
	s.versions["lidar-proc"] = []AppVersion{
		{Version: "0.9.3", PublishedAt: time.Now().Add(-6 * time.Hour), Changelog: "Voxel grid filter tuning"},
		{Version: "0.9.2", PublishedAt: time.Now().Add(-100 * time.Hour), Changelog: "Initial release"},
	}

	s.diagTests = []DiagTest{
		{Name: "lidar-ping", PassedOutput: "LIDAR responding at 10Hz", FailedOutput: "No response from LIDAR", DurationMs: 320, DefaultPassing: true},
		{Name: "camera-check", PassedOutput: "Camera device found at /dev/video0", FailedOutput: "No device found at /dev/video0", DurationMs: 50, DefaultPassing: false},
		{Name: "network-reachability", PassedOutput: "Gateway reachable, latency 2ms", FailedOutput: "Gateway unreachable", DurationMs: 80, DefaultPassing: true},
		{Name: "disk-space", PassedOutput: "Disk usage 42% — OK", FailedOutput: "Disk usage above 90%", DurationMs: 10, DefaultPassing: true},
	}
	for _, t := range s.diagTests {
		output := t.FailedOutput
		if t.DefaultPassing {
			output = t.PassedOutput
		}
		s.diagResults[t.Name] = &DiagResult{
			Name:       t.Name,
			Passed:     t.DefaultPassing,
			Output:     output,
			DurationMs: t.DurationMs,
			RanAt:      time.Now().Add(-5 * time.Minute),
		}
	}

	s.identity = Identity{
		Name:            "robot-01",
		Serial:          "RPi4-ABC123",
		Location:        "warehouse-floor-3",
		FirmwareVersion: "0.4.2",
	}

	// Seed one running instance so the demo has visible state immediately.
	pid := 4821
	s.instances["ros2-nav-1.4.0-demo"] = &Instance{
		ID:        "ros2-nav-1.4.0-demo",
		App:       "ros2-nav",
		Version:   "1.4.0",
		State:     "running",
		StartedAt: time.Now().Add(-10 * time.Minute),
		PID:       &pid,
	}
	s.addEvent("instance_started", map[string]any{
		"instance_id": "ros2-nav-1.4.0-demo",
		"app":         "ros2-nav",
		"version":     "1.4.0",
	})
}

// --- Apps ---

func (s *Store) ListApps() []App {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]App, len(s.apps))
	copy(out, s.apps)
	return out
}

func (s *Store) GetApp(name string) (App, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.apps {
		if a.Name == name {
			return a, true
		}
	}
	return App{}, false
}

func (s *Store) GetVersions(appName string) ([]AppVersion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.versions[appName]
	if !ok {
		return nil, false
	}
	out := make([]AppVersion, len(v))
	copy(out, v)
	return out, true
}

// ResolveVersion returns the concrete version string, handling "latest".
func (s *Store) ResolveVersion(appName, version string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if version == "latest" {
		for _, a := range s.apps {
			if a.Name == appName {
				return a.LatestVersion, true
			}
		}
		return "", false
	}
	for _, v := range s.versions[appName] {
		if v.Version == version {
			return version, true
		}
	}
	return "", false
}

// --- Instances ---

func (s *Store) ListInstances() []*Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		cp := *inst
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetInstance(id string) (*Instance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inst, ok := s.instances[id]
	if !ok {
		return nil, false
	}
	cp := *inst
	return &cp, true
}

func (s *Store) HasRunningInstance(appName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, inst := range s.instances {
		if inst.App == appName && (inst.State == "running" || inst.State == "starting") {
			return true
		}
	}
	return false
}

func (s *Store) CreateInstance(app, version string) *Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	pid := 5000 + len(s.instances)
	id := fmt.Sprintf("%s-%s-%d", app, version, time.Now().Unix())
	inst := &Instance{
		ID:        id,
		App:       app,
		Version:   version,
		State:     "starting",
		StartedAt: time.Now(),
		PID:       &pid,
	}
	s.instances[id] = inst
	return inst
}

func (s *Store) SetInstanceState(id, state string, exitCode *int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.instances[id]
	if !ok {
		return false
	}
	inst.State = state
	if state == "stopped" || state == "crashed" {
		now := time.Now()
		inst.StoppedAt = &now
		inst.ExitCode = exitCode
		inst.PID = nil
	}
	return true
}

// --- Events ---

func (s *Store) AddEvent(eventType string, data map[string]any) Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addEvent(eventType, data)
}

func (s *Store) addEvent(eventType string, data map[string]any) Event {
	s.eventSeq++
	ev := Event{
		ID:        fmt.Sprintf("evt_%04d", s.eventSeq),
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
	s.events = append(s.events, ev)
	if len(s.events) > 500 {
		s.events = s.events[len(s.events)-500:]
	}
	return ev
}

func (s *Store) ListEvents(since *time.Time, eventType string, limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Event
	for i := len(s.events) - 1; i >= 0; i-- {
		ev := s.events[i]
		if since != nil && !ev.Timestamp.After(*since) {
			continue
		}
		if eventType != "" && ev.Type != eventType {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	// Return in chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// --- Diagnostics ---

func (s *Store) ListDiagTests() []DiagTest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DiagTest, len(s.diagTests))
	copy(out, s.diagTests)
	return out
}

func (s *Store) GetDiagTest(name string) (DiagTest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.diagTests {
		if t.Name == name {
			return t, true
		}
	}
	return DiagTest{}, false
}

func (s *Store) GetDiagResult(name string) (*DiagResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.diagResults[name]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

func (s *Store) SetDiagResult(r DiagResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := r
	s.diagResults[r.Name] = &cp
}

// --- Identity ---

func (s *Store) GetIdentity() Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identity
}

func (s *Store) SetLocation(loc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identity.Location = loc
}
