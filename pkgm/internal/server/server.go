package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"firecracker-manager/internal/assets"
	"firecracker-manager/internal/runtime"

	"github.com/containerd/containerd/services/server"
	"github.com/containerd/containerd/sys"
	"github.com/sirupsen/logrus"
)

type Server struct {
	dataDir string
	server  *server.Server
	assets  *assets.Manager
}

func New(dataDir string) (*Server, error) {
	// Create data directory
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	// Extract embedded assets
	assetMgr, err := assets.New(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup assets: %w", err)
	}

	return &Server{
		dataDir: dataDir,
		assets:  assetMgr,
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	// Setup containerd configuration
	config := s.createConfig()

	// Create containerd server
	srv, err := server.New(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create containerd server: %w", err)
	}

	s.server = srv

	// Register firecracker runtime
	err = runtime.Register(s.server, s.assets.FirecrackerShimPath())
	if err != nil {
		return fmt.Errorf("failed to register firecracker runtime: %w", err)
	}

	logrus.Info("Starting embedded containerd server")

	// Start the server (blocking)
	return srv.Serve(ctx)
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
	if s.assets != nil {
		s.assets.Cleanup()
	}
}

func (s *Server) createConfig() *server.Config {
	return &server.Config{
		Version: 2,
		Root:    filepath.Join(s.dataDir, "containerd"),
		State:   filepath.Join(s.dataDir, "run"),
		GRPC: server.GRPCConfig{
			Address: filepath.Join(s.dataDir, "containerd.sock"),
		},
		Debug: server.Debug{
			Level: "info",
		},
		Metrics: server.MetricsConfig{
			Address: "",
		},
		DisabledPlugins: []string{},
		RequiredPlugins: []string{},
		Plugins: map[string]interface{}{
			"io.containerd.grpc.v1.cri": map[string]interface{}{
				"containerd": map[string]interface{}{
					"runtimes": map[string]interface{}{
						"aws.firecracker": map[string]interface{}{
							"runtime_type": "aws.firecracker",
						},
					},
				},
			},
		},
		OOMScore: sys.OOMScoreMaxKillable,
	}
}