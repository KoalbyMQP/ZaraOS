package types

import "context"

// VMManager defines the interface for managing microVMs
type VMManager interface {
	Create(ctx context.Context, vmID, image string) (VM, error)
	Destroy(ctx context.Context, vmID string) error
	List(ctx context.Context) ([]VM, error)
	Get(ctx context.Context, vmID string) (VM, error)
}

// VM represents a running microVM instance
type VM interface {
	ID() string
	Status(ctx context.Context) (string, error)
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
}

// VMSpec defines the specification for creating a microVM
type VMSpec struct {
	ID       string            `json:"id"`
	Image    string            `json:"image"`
	Memory   int64             `json:"memory_mb"`
	CPUs     int64             `json:"cpus"`
	Env      map[string]string `json:"environment"`
	Command  []string          `json:"command"`
	Networks []string          `json:"networks"`
}

// VMStatus represents the current state of a microVM
type VMStatus struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Memory int64  `json:"memory_usage"`
	CPU    int64  `json:"cpu_usage"`
	Uptime int64  `json:"uptime_seconds"`
}
