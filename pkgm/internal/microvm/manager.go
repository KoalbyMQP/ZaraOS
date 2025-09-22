package microvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

type Manager struct {
	dataDir string
	vms     map[string]*firecracker.Machine
}

type VMInfo struct {
	ID     string
	Status string
}

func NewManager(dataDir string) (*Manager, error) {
	// Create data directory
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	return &Manager{
		dataDir: dataDir,
		vms:     make(map[string]*firecracker.Machine),
	}, nil
}

func (m *Manager) Create(ctx context.Context, vmID, image string) error {
	if _, exists := m.vms[vmID]; exists {
		return fmt.Errorf("VM %s already exists", vmID)
	}

	// VM configuration
	socketPath := filepath.Join(m.dataDir, fmt.Sprintf("%s.sock", vmID))
	
	// Use shared rootfs for testing
	rootfsPath := "/var/lib/firecracker/rootfs.ext4"
	
	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "/var/lib/firecracker/vmlinux.bin",
		KernelArgs:      "console=ttyS0 reboot=k panic=1",
		
		Drives: firecracker.NewDrivesBuilder(rootfsPath).Build(),
		
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(1),
			MemSizeMib: firecracker.Int64(128),
		},
		
		NetworkInterfaces: []firecracker.NetworkInterface{},
	}

	// Create machine
	machine, err := firecracker.NewMachine(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create machine: %w", err)
	}

	// Start the machine
	if err := machine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start machine: %w", err)
	}

	m.vms[vmID] = machine
	fmt.Printf("Started microVM %s\n", vmID)
	return nil
}

func (m *Manager) Destroy(ctx context.Context, vmID string) error {
	machine, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	// Stop the machine
	if err := machine.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown machine: %w", err)
	}

	delete(m.vms, vmID)
	return nil
}

func (m *Manager) List(ctx context.Context) ([]VMInfo, error) {
	var vms []VMInfo
	for id, machine := range m.vms {
		status := "running"
		// Simple status check - in reality we'd ping the machine
		if machine == nil {
			status = "stopped"
		}
		
		vms = append(vms, VMInfo{
			ID:     id,
			Status: status,
		})
	}
	return vms, nil
}

func (m *Manager) Close() error {
	// Clean shutdown of all VMs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	for vmID := range m.vms {
		m.Destroy(ctx, vmID)
	}
	return nil
}