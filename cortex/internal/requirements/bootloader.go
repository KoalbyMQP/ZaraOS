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
	controlLoopInterval   = 10 * time.Second
)

// UpdateAvailable describes a package with a newer version in the registry.
type UpdateAvailable struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateReady    bool   `json:"update_available"`
}

// Bootloader orchestrates the startup of required packages in dependency
// order and continuously reconciles the desired state (manifest) against
// the actual state (running containers).
type Bootloader struct {
	store    *store.Store
	registry *registry.Registry
	log      *logger.Logger

	manifestPath string
	manifest     *Manifest

	// Track restart state for exponential backoff.
	mu         sync.Mutex
	restartLog map[string]*restartState
}

// restartState tracks per-package restart history for exponential backoff.
type restartState struct {
	count      int       // total restarts since last stable period
	lastCrash  time.Time // when the last restart was attempted
	stableSince time.Time // when the package last started successfully
}

const (
	// If a package stays running for this long, reset its restart counter.
	// This prevents a package that crashed once 2 hours ago from being
	// permanently penalized.
	stableResetDuration = 5 * time.Minute

	// Base backoff between restart attempts. Doubles each time.
	baseBackoff = 10 * time.Second
)

// NewBootloader creates a bootloader that will load the manifest from the
// given path (falling back to the OS-baked default).
func NewBootloader(s *store.Store, r *registry.Registry, l *logger.Logger, manifestPath string) *Bootloader {
	if manifestPath == "" {
		manifestPath = defaultManifestPath
	}
	return &Bootloader{
		store:        s,
		registry:     r,
		log:          l,
		manifestPath: manifestPath,
		restartLog:   make(map[string]*restartState),
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

// Run loads the manifest and executes the initial boot sequence.
// After the boot sequence, it starts the continuous control loop.
func (b *Bootloader) Run(ctx context.Context) error {
	if err := b.loadManifest(); err != nil {
		return err
	}

	// Initial boot: resolve deps and start everything in order.
	if err := b.reconcileAll(ctx); err != nil {
		b.log.Error("requirements: initial boot had errors", "err", err)
		// Don't return — start the control loop anyway.
	}

	b.log.Info("requirements: boot complete, starting control loop")
	b.controlLoop(ctx)
	return nil
}

// loadManifest reads the manifest from disk.
func (b *Bootloader) loadManifest() error {
	b.log.Info("requirements: loading manifest", "path", b.manifestPath)

	m, err := LoadManifest(b.manifestPath, fallbackManifestPath)
	if err != nil {
		b.log.Error("requirements: failed to load manifest", "err", err)
		return fmt.Errorf("load manifest: %w", err)
	}

	// Recalculate priorities from the dependency graph.
	m.RecalcPriorities()
	b.manifest = m

	b.log.Info("requirements: manifest loaded",
		"version", m.Version,
		"packages", len(m.Packages),
	)
	return nil
}

// controlLoop is the continuous reconciler. Every tick it re-reads the
// manifest (in case it was updated via the API or auto-discovery) and
// ensures every autostart package whose run_condition is met is running.
func (b *Bootloader) controlLoop(ctx context.Context) {
	b.log.Info("requirements: control loop started",
		"interval", controlLoopInterval.String(),
	)
	ticker := time.NewTicker(controlLoopInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("requirements: control loop stopped")
			return
		case <-ticker.C:
			// Re-read manifest from disk in case it was updated.
			if err := b.reloadManifest(); err != nil {
				b.log.Warn("requirements: manifest reload failed, using previous", "err", err)
			}
			// Reconcile desired state vs actual state.
			if err := b.reconcileAll(ctx); err != nil {
				b.log.Debug("requirements: reconcile tick had errors", "err", err)
			}
		case <-cleanupTicker.C:
			// Periodically clean up old stopped containers to prevent
			// the SD card from filling up.
			b.cleanupStoppedContainers()
		}
	}
}

// reloadManifest re-reads the manifest from disk without logging noise
// on every tick. Only updates b.manifest if the file parses successfully.
func (b *Bootloader) reloadManifest() error {
	m, err := LoadManifest(b.manifestPath, fallbackManifestPath)
	if err != nil {
		return err
	}
	m.RecalcPriorities()
	b.manifest = m
	return nil
}

// reconcileAll is the core of the control loop. It resolves the dependency
// order and ensures every autostart package is running.
func (b *Bootloader) reconcileAll(ctx context.Context) error {
	if b.manifest == nil {
		return nil
	}

	autostart := b.manifest.AutostartPackages()
	if len(autostart) == 0 {
		return nil
	}

	tiers, err := Resolve(autostart)
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	var lastErr error
	for _, tier := range tiers {
		var wg sync.WaitGroup
		errs := make([]error, len(tier))

		for i, pkg := range tier {
			wg.Add(1)
			go func(idx int, p Package) {
				defer wg.Done()
				errs[idx] = b.ensureRunning(ctx, p)
			}(i, pkg)
		}
		wg.Wait()

		for i, pkg := range tier {
			if errs[i] != nil {
				lastErr = errs[i]
				// Only log on actual start attempts, not "already running" skips.
				b.log.Error("requirements: package not running",
					"package", pkg.Name,
					"essential", pkg.Essential,
					"err", errs[i],
				)
			}
		}
	}

	return lastErr
}

// ensureRunning checks if a package should be running and starts it if not.
// Uses exponential backoff and a max_restarts budget to avoid crash-loops.
// Resets the restart counter after a package runs stably for 5 minutes.
func (b *Bootloader) ensureRunning(ctx context.Context, pkg Package) error {
	if b.store.HasRunningInstance(pkg.Name) {
		// Running — track stability. If it's been up long enough, reset restarts.
		b.mu.Lock()
		rs := b.restartLog[pkg.Name]
		if rs != nil && !rs.stableSince.IsZero() &&
			time.Since(rs.stableSince) > stableResetDuration && rs.count > 0 {
			b.log.Info("requirements: package stable, resetting restart counter",
				"package", pkg.Name, "was", rs.count,
			)
			rs.count = 0
		}
		b.mu.Unlock()
		return nil
	}

	// Package is not running. Check if health_check allows restarts.
	if pkg.HealthCheck != nil && pkg.HealthCheck.Enabled {
		b.mu.Lock()
		rs := b.restartLog[pkg.Name]
		if rs == nil {
			rs = &restartState{}
			b.restartLog[pkg.Name] = rs
		}

		// Exhausted restart budget.
		if !pkg.HealthCheck.RestartOnFailure || rs.count >= pkg.HealthCheck.MaxRestarts {
			b.mu.Unlock()
			return nil
		}

		// Exponential backoff: wait 10s, 20s, 40s, 80s... between attempts.
		if rs.count > 0 && !rs.lastCrash.IsZero() {
			backoff := baseBackoff * time.Duration(1<<uint(rs.count-1))
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute // cap at 5 min
			}
			if time.Since(rs.lastCrash) < backoff {
				b.mu.Unlock()
				return nil // too soon, wait for backoff
			}
		}

		rs.count++
		rs.lastCrash = time.Now()
		count := rs.count
		b.mu.Unlock()

		if count > 1 {
			b.log.Warn("requirements: restarting package (backoff)",
				"package", pkg.Name,
				"restart_count", count,
				"max_restarts", pkg.HealthCheck.MaxRestarts,
			)
		}
	}

	err := b.startPackage(ctx, pkg)
	if err != nil {
		b.store.AddEvent("requirement_start_failed", map[string]any{
			"package":   pkg.Name,
			"essential": pkg.Essential,
			"error":     err.Error(),
		})
		return err
	}

	// Mark stable start time for reset tracking.
	b.mu.Lock()
	rs := b.restartLog[pkg.Name]
	if rs == nil {
		rs = &restartState{}
		b.restartLog[pkg.Name] = rs
	}
	rs.stableSince = time.Now()
	b.mu.Unlock()

	b.log.Event("REQUIREMENT RUNNING", "package", pkg.Name)
	return nil
}

// startPackage handles the full lifecycle of starting a single package:
// resolve version, check local image, optionally pull, run container.
func (b *Bootloader) startPackage(ctx context.Context, pkg Package) error {
	// Resolve version.
	version := pkg.Version
	if version == "latest" {
		if v := b.registry.LatestVersion(pkg.Name); v != "" {
			version = v
		} else {
			version = "latest"
		}
	}

	imageRef := pkg.ImageRef(version)

	// Check if image exists locally.
	localAvailable := b.imageExistsLocally(imageRef)

	// If image not local or version is "latest", try to pull.
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

	if err := b.runContainer(inst.ID, inst.ContainerID, imageRef, pkg); err != nil {
		exitCode := 1
		b.store.SetInstanceState(inst.ID, "crashed", &exitCode)
		b.store.SetInstanceError(inst.ID, err.Error())
		return fmt.Errorf("container start failed: %w", err)
	}

	b.store.SetInstanceState(inst.ID, "running", nil)

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

// --- Container helpers ---

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

func (b *Bootloader) runContainer(instanceID, containerID, imageRef string, pkg Package) error {
	args := []string{"run", "-d", "--name", containerID, "--network", "host"}

	// Privileged mode grants full access to the host's devices.
	if pkg.Privileged {
		args = append(args, "--privileged")
	}

	// Explicit device passthrough (e.g. "/dev/i2c-1", "/dev/video0").
	for _, dev := range pkg.Devices {
		args = append(args, "--device", dev)
	}

	// Volume mounts (e.g. "/dev:/dev", "/sys:/sys:ro").
	for _, vol := range pkg.Volumes {
		args = append(args, "-v", vol)
	}

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

// --- Updates ---

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

		for _, inst := range b.store.ListInstances() {
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

// cleanupStoppedContainers removes old stopped/crashed containers that
// are managed by the requirements system. Prevents the SD card from
// filling up with dead container state.
func (b *Bootloader) cleanupStoppedContainers() {
	instances := b.store.ListInstances()
	for _, inst := range instances {
		if !b.store.IsPackageManaged(inst.App) {
			continue
		}
		if inst.State != "stopped" && inst.State != "crashed" {
			continue
		}
		// Only clean up containers that stopped more than 2 minutes ago
		// (give recently-stopped ones time to be inspected).
		if inst.StoppedAt == nil || time.Since(*inst.StoppedAt) < 2*time.Minute {
			continue
		}

		exec.Command(containerCLI(), "rm", inst.ContainerID).Run()
		b.log.Debug("requirements: cleaned up container",
			"package", inst.App, "container", inst.ContainerID,
		)
	}
}

// containerCLI returns the container runtime CLI command.
func containerCLI() string {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("nerdctl"); err == nil {
		return "nerdctl"
	}
	return "docker"
}
