package store

import "testing"

// A transition back to "running" must not keep the error, stop time or exit
// code from an earlier terminal state (e.g. a reconciler race during start).
func TestSetInstanceStateRunningClearsTerminalFields(t *testing.T) {
	s := New()
	inst := s.CreateInstance("app", "v1", "img")

	exitCode := 1
	s.SetInstanceState(inst.ID, "crashed", &exitCode)
	s.SetInstanceError(inst.ID, "container exited: exit status 1")
	s.SetInstanceState(inst.ID, "running", nil)

	got, _ := s.GetInstance(inst.ID)
	if got.State != "running" {
		t.Fatalf("State = %q, want running", got.State)
	}
	if got.StoppedAt != nil {
		t.Errorf("StoppedAt = %v, want nil", got.StoppedAt)
	}
	if got.ExitCode != nil {
		t.Errorf("ExitCode = %d, want nil", *got.ExitCode)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

func TestSetInstanceStateCrashedRecordsTerminalFields(t *testing.T) {
	s := New()
	inst := s.CreateInstance("app", "v1", "img")

	exitCode := 1
	s.SetInstanceState(inst.ID, "crashed", &exitCode)

	got, _ := s.GetInstance(inst.ID)
	if got.StoppedAt == nil {
		t.Error("StoppedAt = nil, want set")
	}
	if got.ExitCode == nil || *got.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", got.ExitCode)
	}
}
