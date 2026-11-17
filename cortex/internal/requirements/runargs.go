package requirements

import (
	"context"
	"sort"
)

// IPC modes accepted by the manifest "ipc" field (passed to --ipc).
const (
	IPCHost      = "host"      // share the host IPC namespace + /dev/shm (default)
	IPCPrivate   = "private"   // own IPC namespace, not shareable
	IPCShareable = "shareable" // own IPC namespace, joinable by others
)

// Shared-memory (iceoryx / CycloneDDS) wiring. These paths must match the
// ZaraOS rootfs overlay (usr/bin/zaraos-roudi.sh, etc/zaraos/cyclonedds-shm.xml).
const (
	// IceoryxSocketDir is the host directory RouDi sees as /tmp. iceoryx 2.0
	// hardcodes its unix sockets under /tmp/, so shared-memory containers get
	// this directory mounted at /tmp.
	IceoryxSocketDir = "/tmp/iceoryx"

	// CycloneSHMConfigHost / CycloneSHMConfigContainer is the CycloneDDS
	// config enabling the iceoryx transport, and where it lands in the container.
	CycloneSHMConfigHost      = "/etc/zaraos/cyclonedds-shm.xml"
	CycloneSHMConfigContainer = "/etc/cyclonedds-shm.xml"
)

// ValidIPC reports whether mode is an accepted manifest "ipc" value.
// The empty string means "use the default" (host).
func ValidIPC(mode string) bool {
	switch mode {
	case "", IPCHost, IPCPrivate, IPCShareable:
		return true
	}
	return false
}

// RunOpts carries the runtime flags used to start an app container. It is
// built from a manifest Package and/or an API request.
type RunOpts struct {
	Privileged bool
	Devices    []string
	Volumes    []string
	Env        map[string]string

	// IPC is the --ipc mode; empty means IPCHost.
	IPC string

	// SharedMemory wires the container up for CycloneDDS shared memory via
	// iceoryx: forces --ipc host, mounts the RouDi socket dir at /tmp, and
	// points CYCLONEDDS_URI at the SHM config (unless Env already sets it).
	// RouDi must be running first — see Bootloader.PrepareRun.
	SharedMemory bool
}

// RunOptsFromPackage returns the RunOpts declared by a manifest package.
func RunOptsFromPackage(p Package) RunOpts {
	return RunOpts{
		Privileged:   p.Privileged,
		Devices:      p.Devices,
		Volumes:      p.Volumes,
		Env:          p.Env,
		IPC:          p.IPC,
		SharedMemory: p.SharedMemory,
	}
}

// EffectiveIPC returns the --ipc mode that will actually be used.
func (o RunOpts) EffectiveIPC() string {
	if o.SharedMemory || o.IPC == "" {
		return IPCHost
	}
	return o.IPC
}

// BuildRunArgs returns the container CLI arguments ("run -d ...") that start
// image as containerID. Every app container shares the host network; IPC is
// shared with the host unless the package opts out.
func BuildRunArgs(containerID, image string, o RunOpts) []string {
	args := []string{
		"run", "-d", "--name", containerID,
		"--network", "host",
		"--ipc", o.EffectiveIPC(),
	}

	if o.Privileged {
		args = append(args, "--privileged")
	}
	for _, dev := range o.Devices {
		args = append(args, "--device", dev)
	}
	for _, vol := range o.Volumes {
		args = append(args, "-v", vol)
	}

	env := make(map[string]string, len(o.Env)+1)
	for k, v := range o.Env {
		env[k] = v
	}
	if o.SharedMemory {
		args = append(args,
			"-v", IceoryxSocketDir+":/tmp",
			"-v", CycloneSHMConfigHost+":"+CycloneSHMConfigContainer+":ro",
		)
		if _, set := env["CYCLONEDDS_URI"]; !set {
			env["CYCLONEDDS_URI"] = "file://" + CycloneSHMConfigContainer
		}
	}

	// Sorted for deterministic output (logs, tests).
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}

	return append(args, image)
}

// SharedMemoryProvider makes sure the shared-memory broker (iceoryx RouDi)
// is running before a shared_memory container starts. Implementations track
// instanceID so the broker can be stopped once no user is left.
type SharedMemoryProvider interface {
	Acquire(ctx context.Context, instanceID string) error
}
