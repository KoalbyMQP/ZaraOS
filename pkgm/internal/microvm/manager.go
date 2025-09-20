package vm

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	defaultNamespace = "firecracker-all"
	runtimeName      = "aws.firecracker"
)

type Manager struct {
	client *containerd.Client
}

type VMInfo struct {
	ID     string
	Status string
}

func NewManager(dataDir string) (*Manager, error) {
	socketPath := filepath.Join(dataDir, "containerd.sock")

	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}

	return &Manager{
		client: client,
	}, nil
}

func (m *Manager) Create(ctx context.Context, vmID, image string) error {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	// Check if container already exists
	_, err := m.client.LoadContainer(ctx, vmID)
	if err == nil {
		return fmt.Errorf("VM %s already exists", vmID)
	}

	// Pull image
	img, err := m.client.Pull(ctx, image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	// Create container
	container, err := m.client.NewContainer(
		ctx,
		vmID,
		containerd.WithImage(img),
		containerd.WithNewSnapshot(vmID+"-snapshot", img),
		containerd.WithNewSpec(
			oci.WithImageConfig(img),
			oci.WithProcessArgs("/bin/sh", "-c", "echo 'VM started'; sleep 300"),
			withFirecrackerConfig(),
		),
		containerd.WithRuntime(runtimeName, nil),
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Create and start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Start the task
	err = task.Start(ctx)
	if err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return fmt.Errorf("failed to start task: %w", err)
	}

	return nil
}

func (m *Manager) Destroy(ctx context.Context, vmID string) error {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	// Load container
	container, err := m.client.LoadContainer(ctx, vmID)
	if err != nil {
		return fmt.Errorf("VM %s not found: %w", vmID, err)
	}

	// Get task
	task, err := container.Task(ctx, nil)
	if err == nil {
		// Kill task
		err = task.Kill(ctx, 9)
		if err != nil {
			return fmt.Errorf("failed to kill task: %w", err)
		}

		// Wait for task to exit
		exitCh, err := task.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to wait for task: %w", err)
		}

		select {
		case <-exitCh:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for task to exit")
		}

		// Delete task
		_, err = task.Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}
	}

	// Delete container
	err = container.Delete(ctx, containerd.WithSnapshotCleanup)
	if err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return nil
}

func (m *Manager) List(ctx context.Context) ([]VMInfo, error) {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	containers, err := m.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var vms []VMInfo
	for _, container := range containers {
		status := "stopped"

		task, err := container.Task(ctx, nil)
		if err == nil {
			taskStatus, err := task.Status(ctx)
			if err == nil {
				status = string(taskStatus.Status)
			}
		}

		vms = append(vms, VMInfo{
			ID:     container.ID(),
			Status: status,
		})
	}

	return vms, nil
}

func (m *Manager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

func withFirecrackerConfig() oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, container *containerd.Container, s *specs.Spec) error {
		// Configure for firecracker
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}

		// Set memory limit (128MB)
		if s.Linux.Resources == nil {
			s.Linux.Resources = &specs.LinuxResources{}
		}
		if s.Linux.Resources.Memory == nil {
			s.Linux.Resources.Memory = &specs.LinuxMemory{}
		}
		limit := int64(128 * 1024 * 1024)
		s.Linux.Resources.Memory.Limit = &limit

		return nil
	}
}
