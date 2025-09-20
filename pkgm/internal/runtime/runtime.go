package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/services/server"
	"github.com/sirupsen/logrus"
)

const (
	runtimeName = "aws.firecracker"
)

// Register configures the firecracker runtime with containerd
func Register(srv *server.Server, shimPath string) error {
	// Ensure shim is executable and in PATH
	if err := setupShim(shimPath); err != nil {
		return fmt.Errorf("failed to setup firecracker shim: %w", err)
	}

	logrus.Infof("Registered firecracker runtime with shim: %s", shimPath)
	return nil
}

func setupShim(shimPath string) error {
	// Verify shim exists and is executable
	info, err := os.Stat(shimPath)
	if err != nil {
		return fmt.Errorf("shim not found: %w", err)
	}

	if info.Mode()&0111 == 0 {
		return fmt.Errorf("shim is not executable: %s", shimPath)
	}

	// Add shim directory to PATH so containerd can find it
	shimDir := filepath.Dir(shimPath)
	currentPath := os.Getenv("PATH")
	newPath := shimDir + ":" + currentPath
	os.Setenv("PATH", newPath)

	return nil
}
