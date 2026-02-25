package store

import (
	"fmt"
	"sync"
	"time"
)

// --- Instance ---

type Instance struct {
	ID          string
	App         string
	Version     string
	Image       string
	ContainerID string // nerdctl container name (== ID)
	State       string // starting | running | stopping | stopped | crashed
	Error       string // set on async failure
	StartedAt   time.Time
	StoppedAt   *time.Time
	PID         *int
	ExitCode    *int
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

	instances   map[string]*Instance
	events      []Event
	eventSeq    int
	diagTests   []DiagTest
	diagResults map[string]*DiagResult
	identity    Identity
}

func New() *Store {
	s := &Store{
		instances:   make(map[string]*Instance),
		diagResults: make(map[string]*DiagResult),
	}
	s.seed()
	return s
}

func (s *Store) seed() {
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

func (s *Store) CreateInstance(app, version, image string) *Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("%s-%s-%d", app, version, time.Now().Unix())
	inst := &Instance{
		ID:          id,
		App:         app,
		Version:     version,
		Image:       image,
		ContainerID: id, // container name == instance ID
		State:       "starting",
		StartedAt:   time.Now(),
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

func (s *Store) SetInstanceError(id, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inst, ok := s.instances[id]; ok {
		inst.Error = msg
	}
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
