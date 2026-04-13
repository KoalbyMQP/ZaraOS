package requirements

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cortex/internal/logger"
	"cortex/internal/registry"
	"cortex/internal/store"
)

const (
	defaultManifestPath  = "/data/config/requirements.json"
	fallbackManifestPath = "/etc/zaraos/requirements.json"

	containerStartTimeout = 30 * time.Second
	imagePullTimeout      = 120 * time.Second
	healthCheckInterval   = 10 * time.Second
)

// UpdateAvailable describes a package with a newer version in the registry.
type UpdateAvailable struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateReady    bool   `json:"update_available"`
}

// Bootloader orchestrates the startup of required packages in dependency order.
type Bootloader struct {
	store    *store.Store
	registry *registry.Registry
	log      *logger.Logger

	manifestPath string
	manifest     *Manifest

	// Track restart counts for health monitoring.
	mu            sync.Mutex
	restartCounts map[string]int // package name -> restart count
}

// NewBootloader creates a bootloader that will load the manifest from the
// given path (falling back to the OS-baked default).
func NewBootloader(s *store.Store, r *registry.Registry, l *logger.Logger, manifestPath string) *Bootloader {
	if manifestPath == "" {
		manifestPath = defaultManifestPath
	}
	return &Bootloader{
		store:         s,
		registry:      r,
		log:           l,
		manifestPath:  manifestPath,
		restartCounts: make(map[string]int),
	}
}

// Manifest returns the currently loaded manifest (may be nil before Run).
func (b *Bootloader) Manifest() *Manifest {
	return b.manifest
}

// ManifestPath returns the path the manifest was loaded from.
func (b *Bootloader) ManifestPath() string {
	return b.manifestPath
}

// Run executes the full boot sequence: load manifest, resolve deps, start packages.
func (b *Bootloader) Run(ctx context.Context) error {
	b.log.Info("requirements: loading manifest", "path", b.manifestPath)

	m, err := LoadManifest(b.manifestPath, fallbackManifestPath)
	if err != nil {
		b.log.Error("requirements: failed to load manifest", "err", err)
		return fmt.Errorf("load manifest: %w", err)
	}
	b.manifest = m

	b.log.Info("requirements: manifest loaded",
		"version", m.Version,
		"packages", len(m.Packages),
	)

	// Filter to autostart packages.
	autostart := m.AutostartPackages()
	if len(autostart) == 0 {
		b.log.Info("requirements: no autostart packages")
		return nil
	}

	// Resolve dependency order.
	tiers, err := Resolve(autostart)
	if err != nil {
		b.log.Error("requirements: dependency resolution failed", "err", err)
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	b.log.Info("requirements: starting boot sequence",
		"tiers", len(tiers),
		"autostart", len(autostart),
	)

	// Execute tiers sequentially; packages within a tier start in parallel.
	for tierIdx, tier := range tiers {
		b.log.Info("requirements: starting tier",
			"tier", tierIdx,
			"packages", len(tier),
		)

		var wg sync.WaitGroup
		errs := make([]error, len(tier))

		for i, pkg := range tier {
			wg.Add(1)
			go func(idx int, p Package) {
				defer wg.Done()
				errs[idx] = b.startPackage(ctx, p)
			}(i, pkg)
		}
		wg.Wait()

		// Log results for this tier.
		for i, pkg := range tier {
			if errs[i] != nil {
				b.log.Error("requirements: package failed to start",
					"package", pkg.Name,
					"essential", pkg.Essential,
					"err", errs[i],
				)
				b.store.AddEvent("requirement_start_failed", map[string]any{
					"package":   pkg.Name,
					"essential": pkg.Essential,
					"error":     errs[i].Error(),
				})
			} else {
				b.log.Event("REQUIREMENT STARTED", "package", pkg.Name)
			}
		}
	}

	b.log.Info("requirements: boot sequence complete")
	return nil
}

// startPackage handles the full lifecycle of starting a single package:
// resolve version, check local image, optionally pull update, run container.
func (b *Bootloader) startPackage(ctx context.Context, pkg Package) error {
	// Check if already running (from a previous boot or manual start).
	if b.store.HasRunningInstance(pkg.Name) {
		b.log.Info("requirements: already running, skipping", "package", pkg.Name)
		return nil
	}

	// Resolve version.
	version := pkg.Version
	if version == "latest" {
		if v := b.registry.LatestVersion(pkg.Name); v != "" {
			version = v
		} else {
			// Fall back to whatever local image is available.
			version = "latest"
		}
	}

	imageRef := pkg.ImageRef(version)

	// Check if image exists locally.
	localAvailable := b.imageExistsLocally(imageRef)

	// If version is "latest" or we want to check for updates, try to pull.
	if !localAvailable || pkg.Version == "latest" {
		b.log.Info("requirements: pulling image", "package", pkg.Name, "image", imageRef)
		if err := b.pullImage(ctx, imageRef); err != nil {
			if localAvailable {
				b.log.Warn("requirements: pull failed, using cached image",
					"package", pkg.Name, "err", err,
				)
			} else {
				return fmt.Errorf("image not available locally and pull failed: %w", err)
			}
		}
	}

	// Create instance in store and run container.
	inst := b.store.CreateInstance(pkg.Name, version, imageRef)
	b.store.SetPackageManaged(pkg.Name, true)

	b.log.Info("requirements: starting container",
		"package", pkg.Name,
		"version", version,
		"instance", inst.ID,
	)

	// Run with environment variables.
	if err := b.runContainer(inst.ID, inst.ContainerID, imageRef, pkg); err != nil {
		exitCode := 1
		b.store.SetInstanceState(inst.ID, "crashed", &exitCode)
		b.store.SetInstanceError(inst.ID, err.Error())
		return fmt.Errorf("container start failed: %w", err)
	}

	b.store.SetInstanceState(inst.ID, "running", nil)

	// Wait for the container to be confirmed running.
	if err := b.waitForRunning(ctx, inst.ContainerID); err != nil {
		return fmt.Errorf("container did not become healthy: %w", err)
	}

	b.store.AddEvent("requirement_started", map[string]any{
		"package":  pkg.Name,
		"version":  version,
		"instance": inst.ID,
	})

	return nil
}

// imageExistsLocally checks whether a container image is cached on disk.
func (b *Bootloader) imageExistsLocally(imageRef string) bool {
	out, err := exec.Command(containerCLI(), "images", "--format", "{{.Repository}}:{{.Tag}}").
		CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == imageRef {
			return true
		}
	}
	return false
}

// pullImage pulls a container image with a timeout.
func (b *Bootloader) pullImage(ctx context.Context, imageRef string) error {
	pullCtx, cancel := context.WithTimeout(ctx, imagePullTimeout)
	defer cancel()

	out, err := exec.CommandContext(pullCtx, containerCLI(), "pull", imageRef).
		CombinedOutput()
	if err != nil {
		return fmt.Errorf("pull %s: %s — %w", imageRef, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// runContainer starts a container with the package's environment variables.
func (b *Bootloader) runContainer(instanceID, containerID, imageRef string, pkg Package) error {
	args := []string{"run", "-d", "--name", containerID, "--network", "host"}

	// Add environment variables from the package config.
	for k, v := range pkg.Env {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, imageRef)

	out, err := exec.Command(containerCLI(), args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s — %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// waitForRunning polls the container state until it's confirmed running
// or the timeout expires.
func (b *Bootloader) waitForRunning(ctx context.Context, containerID string) error {
	deadline := time.Now().Add(containerStartTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := exec.Command(containerCLI(), "inspect", "--format", "{{.State.Running}}", containerID).
			CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for container %s to start", containerID)
}

// StartHealthMonitor runs a background loop that watches essential packages
// and restarts them if they crash (up to max_restarts per package).
func (b *Bootloader) StartHealthMonitor(ctx context.Context) {
	if b.manifest == nil {
		return
	}

	b.log.Info("requirements: health monitor started")
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("requirements: health monitor stopped")
			return
		case <-ticker.C:
			b.checkHealth(ctx)
		}
	}
}

// checkHealth examines each essential autostart package and restarts
// crashed ones if their health_check policy allows.
func (b *Bootloader) checkHealth(ctx context.Context) {
	if b.manifest == nil {
		return
	}

	for _, pkg := range b.manifest.Packages {
		if !pkg.Essential || !pkg.Autostart {
			continue
		}
		if !EvalRunCondition(pkg.RunCondition) {
			continue
		}
		if pkg.HealthCheck == nil || !pkg.HealthCheck.Enabled || !pkg.HealthCheck.RestartOnFailure {
			continue
		}

		// Check if any instance of this package is running.
		if b.store.HasRunningInstance(pkg.Name) {
			continue
		}

		// Package should be running but isn't — check restart budget.
		b.mu.Lock()
		count := b.restartCounts[pkg.Name]
		if count >= pkg.HealthCheck.MaxRestarts {
			b.mu.Unlock()
			continue
		}
		b.restartCounts[pkg.Name] = count + 1
		b.mu.Unlock()

		b.log.Warn("requirements: essential package not running, restarting",
			"package", pkg.Name,
			"restart_count", count+1,
			"max_restarts", pkg.HealthCheck.MaxRestarts,
		)

		go func(p Package) {
			if err := b.startPackage(ctx, p); err != nil {
				b.log.Error("requirements: restart failed",
					"package", p.Name,
					"err", err,
				)
				b.store.AddEvent("requirement_restart_failed", map[string]any{
					"package": p.Name,
					"error":   err.Error(),
				})
			} else {
				b.log.Event("REQUIREMENT RESTARTED", "package", p.Name)
				b.store.AddEvent("requirement_restarted", map[string]any{
					"package": p.Name,
				})
			}
		}(pkg)
	}
}

// CheckUpdates compares running package versions against the latest
// available in the registry.
func (b *Bootloader) CheckUpdates() []UpdateAvailable {
	if b.manifest == nil {
		return nil
	}

	var updates []UpdateAvailable
	for _, pkg := range b.manifest.Packages {
		latest := b.registry.LatestVersion(pkg.Name)
		current := pkg.Version

		// Find actual running version from instances.
		instances := b.store.ListInstances()
		for _, inst := range instances {
			if inst.App == pkg.Name && (inst.State == "running" || inst.State == "starting") {
				current = inst.Version
				break
			}
		}

		u := UpdateAvailable{
			Name:           pkg.Name,
			CurrentVersion: current,
			LatestVersion:  latest,
			UpdateReady:    latest != "" && latest != current && current != "latest",
		}
		updates = append(updates, u)
	}
	return updates
}

// containerCLI returns the container runtime CLI command.
// Checks for docker first, falls back to nerdctl.
func containerCLI() string {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("nerdctl"); err == nil {
		return "nerdctl"
	}
	return "docker"
}
