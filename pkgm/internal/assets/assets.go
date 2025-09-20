package assets

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed firecracker
var firecrackerBinary []byte

//go:embed containerd-shim-aws-firecracker
var firecrackerShim []byte

type Manager struct {
	dataDir          string
	firecrackerPath  string
	shimPath         string
	extracted        bool
}

func New(dataDir string) (*Manager, error) {
	assetsDir := filepath.Join(dataDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create assets dir: %w", err)
	}

	mgr := &Manager{
		dataDir:         dataDir,
		firecrackerPath: filepath.Join(assetsDir, "firecracker"),
		shimPath:        filepath.Join(assetsDir, "containerd-shim-aws-firecracker"),
	}

	if err := mgr.extractAssets(); err != nil {
		return nil, fmt.Errorf("failed to extract assets: %w", err)
	}

	return mgr, nil
}

func (m *Manager) extractAssets() error {
	// Extract firecracker binary
	if err := m.writeExecutable(m.firecrackerPath, firecrackerBinary); err != nil {
		return fmt.Errorf("failed to extract firecracker: %w", err)
	}

	// Extract firecracker shim
	if err := m.writeExecutable(m.shimPath, firecrackerShim); err != nil {
		return fmt.Errorf("failed to extract firecracker shim: %w", err)
	}

	m.extracted = true
	return nil
}

func (m *Manager) writeExecutable(path string, data []byte) error {
	// Write binary data
	if err := os.WriteFile(path, data, 0755); err != nil {
		return err
	}

	// Ensure executable
	return os.Chmod(path, 0755)
}

func (m *Manager) FirecrackerPath() string {
	return m.firecrackerPath
}

func (m *Manager) FirecrackerShimPath() string {
	return m.shimPath
}

func (m *Manager) Cleanup() {
	if m.extracted {
		os.RemoveAll(filepath.Join(m.dataDir, "assets"))
	}
}
