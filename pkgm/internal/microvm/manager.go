package microvm

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	defaultNamespace = "firecracker-manager"
	defaultRuntime   = "aws.firecracker"
	socketPath       = "/run/containerd/containerd.sock"
)

type Manager struct {
	client *containerd.Client
	vms    map[string]*MicroVM
}

type MicroVM struct {
	id        string
	container containerd.Container
	task      containerd.Task
}

func NewManager() (*Manager, error) {
	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}

	return &Manager{
		client: client,
		vms:    make(map[string]*MicroVM),
	}, nil
}

func (m *Manager) Create(ctx context.Context, vmID, image string) (*MicroVM, error) {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	// Check if VM already exists
	if _, exists := m.vms[vmID]; exists {
		return nil, fmt.Errorf("microVM %s already exists", vmID)
	}

	// Pull image if not present
	img, err := m.client.Pull(ctx, image, containerd.WithPullUnpack)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	// Create container with firecracker runtime
	container, err := m.client.NewContainer(
		ctx,
		vmID,
		containerd.WithImage(img),
		containerd.WithNewSnapshot(vmID+"-snapshot", img),
		containerd.WithNewSpec(
			oci.WithImageConfig(img),
			oci.WithProcessArgs("/bin/sh", "-c", "echo 'MicroVM started'; sleep 300"),
			withFirecrackerRuntime(),
		),
		containerd.WithRuntime(defaultRuntime, nil),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create and start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Start the task
	err = task.Start(ctx)
	if err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	vm := &MicroVM{
		id:        vmID,
		container: container,
		task:      task,
	}

	m.vms[vmID] = vm
	return vm, nil
}

func (m *Manager) Destroy(ctx context.Context, vmID string) error {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	vm, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("microVM %s not found", vmID)
	}

	// Kill task
	if vm.task != nil {
		err := vm.task.Kill(ctx, 9)
		if err != nil {
			return fmt.Errorf("failed to kill task: %w", err)
		}

		// Wait for task to exit
		exitCh, err := vm.task.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to wait for task: %w", err)
		}

		select {
		case <-exitCh:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for task to exit")
		}

		// Delete task
		_, err = vm.task.Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}
	}

	// Delete container
	if vm.container != nil {
		err := vm.container.Delete(ctx, containerd.WithSnapshotCleanup)
		if err != nil {
			return fmt.Errorf("failed to delete container: %w", err)
		}
	}

	delete(m.vms, vmID)
	return nil
}

func (m *Manager) Close() error {
	return m.client.Close()
}

func (vm *MicroVM) ID() string {
	return vm.id
}

func (vm *MicroVM) Status(ctx context.Context) (string, error) {
	if vm.task == nil {
		return "stopped", nil
	}

	status, err := vm.task.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get task status: %w", err)
	}

	return string(status.Status), nil
}

func withFirecrackerRuntime() oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, container *containerd.Container, s *specs.Spec) error {
		// Basic firecracker-specific configuration
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}

		// Set memory limit (128MB for testing)
		if s.Linux.Resources == nil {
			s.Linux.Resources = &specs.LinuxResources{}
		}
		if s.Linux.Resources.Memory == nil {
			s.Linux.Resources.Memory = &specs.LinuxMemory{}
		}
		limit := int64(128 * 1024 * 1024) // 128MB
		s.Linux.Resources.Memory.Limit = &limit

		return nil
	}
}
