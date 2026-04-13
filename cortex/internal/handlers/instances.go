package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"cortex/internal/logger"
	"cortex/internal/registry"
	"cortex/internal/requirements"
	"cortex/internal/store"
)

// --- Container CLI auto-detection ---

var (
	cliOnce sync.Once
	cliName string
)

// ContainerCLI returns the detected container CLI command ("docker" or "nerdctl").
// It checks the CONTAINER_CLI env var first, then auto-detects by looking for
// docker (preferred) and nerdctl in PATH.
func ContainerCLI() string {
	cliOnce.Do(func() {
		if v := os.Getenv("CONTAINER_CLI"); v != "" {
			cliName = v
			return
		}
		if _, err := exec.LookPath("docker"); err == nil {
			cliName = "docker"
			return
		}
		if _, err := exec.LookPath("nerdctl"); err == nil {
			cliName = "nerdctl"
			return
		}
		cliName = "docker" // fallback
	})
	return cliName
}

// containerExec runs a container CLI command and returns combined stdout+stderr output.
func containerExec(args ...string) (string, error) {
	out, err := exec.Command(ContainerCLI(), args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// logEvent is a structured SSE log payload.
type logEvent struct {
	Timestamp string `json:"ts"`
	Seq       int64  `json:"seq"`
	Stream    string `json:"stream"`
	Content   string `json:"content"`
}

// taggedLine carries a log line with its source stream label.
type taggedLine struct {
	stream string // "stdout", "stderr", or "heartbeat"
	line   string
}

type InstancesHandler struct {
	store      *store.Store
	registry   *registry.Registry
	log        *logger.Logger
	bootloader *requirements.Bootloader // set after construction via SetBootloader
}

func NewInstancesHandler(s *store.Store, r *registry.Registry, l *logger.Logger) *InstancesHandler {
	return &InstancesHandler{store: s, registry: r, log: l}
}

// SetBootloader wires the requirements bootloader so POST /instances can
// auto-discover zaraos.json from newly deployed images.
func (h *InstancesHandler) SetBootloader(bl *requirements.Bootloader) {
	h.bootloader = bl
}

func (h *InstancesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /instances", h.List)
	mux.HandleFunc("POST /instances", h.Start)
	mux.HandleFunc("GET /instances/{id}", h.Get)
	mux.HandleFunc("DELETE /instances/{id}", h.Stop)
	mux.HandleFunc("POST /instances/{id}/restart", h.Restart)
	mux.HandleFunc("POST /instances/{id}/update", h.Update)
	mux.HandleFunc("GET /instances/{id}/logs", h.Logs)
	mux.HandleFunc("GET /instances/{id}/health", h.Health)
	mux.HandleFunc("GET /instances/{id}/metrics", h.Metrics)
}

func (h *InstancesHandler) List(w http.ResponseWriter, r *http.Request) {
	instances := h.store.ListInstances()
	out := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		h.reconcileInstanceState(inst)
		out = append(out, instanceSummary(inst))
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": out})
}

func (h *InstancesHandler) Get(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.store.GetInstance(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	h.reconcileInstanceState(inst)
	writeJSON(w, http.StatusOK, instanceFull(inst))
}

func (h *InstancesHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App     string `json:"app"`
		Version string `json:"version"`
		Image   string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.App == "" {
		writeError(w, http.StatusBadRequest, "missing required field: app")
		return
	}

	// Reconcile stale state before checking for running instances,
	// so we don't falsely block a new start due to an already-exited container.
	h.reconcileAll()

	if h.store.HasRunningInstance(req.App) {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("an instance of %s is already running", req.App))
		return
	}

	var imageRef, version string

	if req.Image != "" {
		// Local image override: skip registry lookup entirely.
		imageRef = req.Image
		version = req.Version
		if version == "" {
			version = "dev"
		}
	} else {
		if req.Version == "" {
			req.Version = "latest"
		}

		app, ok := h.registry.GetApp(req.App)
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("app %s not found", req.App))
			return
		}

		version, ok = h.registry.ResolveVersion(req.App, req.Version)
		if !ok {
			writeError(w, http.StatusNotFound,
				fmt.Sprintf("version %s not found for app %s", req.Version, req.App))
			return
		}

		imageRef = h.registry.ImageRef(app.Name, version)
	}

	inst := h.store.CreateInstance(req.App, version, imageRef)

	h.log.Event("INSTANCE STARTING", "app", req.App, "version", version, "id", inst.ID, "image", imageRef)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": inst.ID,
		"app":         req.App,
		"version":     version,
		"image":       imageRef,
	})

	h.runContainer(inst.ID, inst.ContainerID, imageRef, req.App, version)

	// Auto-discover: try to read zaraos.json from the image and merge
	// it into the requirements manifest so dependencies are tracked
	// without any extra steps from the user.
	if h.bootloader != nil {
		go h.autoDiscover(imageRef, version)
	}

	inst, _ = h.store.GetInstance(inst.ID)
	writeJSON(w, http.StatusCreated, instanceSummary(inst))
}

// autoDiscover reads the embedded zaraos.json from a container image
// and merges it into the active requirements manifest. Runs in the
// background so it doesn't slow down the POST /instances response.
func (h *InstancesHandler) autoDiscover(imageRef, version string) {
	meta, err := requirements.ReadPackageMeta(imageRef)
	if err != nil {
		// No zaraos.json in image — that's fine, not all images have one.
		h.log.Debug("auto-discover: no zaraos.json", "image", imageRef, "err", err)
		return
	}

	m := h.bootloader.Manifest()
	if m == nil {
		return
	}

	pkg := meta.ToPackage(version)
	if err := m.MergePackage(pkg); err != nil {
		h.log.Warn("auto-discover: merge failed", "package", meta.Name, "err", err)
		return
	}

	if err := m.Save(h.bootloader.ManifestPath()); err != nil {
		h.log.Warn("auto-discover: save failed", "err", err)
		return
	}

	h.log.Event("AUTO-DISCOVERED", "package", meta.Name, "depends", meta.Depends)
	h.store.AddEvent("package_discovered", map[string]any{
		"package": meta.Name,
		"image":   imageRef,
		"version": version,
		"depends": meta.Depends,
	})
}

func (h *InstancesHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	h.store.SetInstanceState(id, "stopping", nil)
	h.log.Event("INSTANCE STOPPING", "id", id, "app", inst.App)

	h.stopContainer(id, inst.ContainerID, inst.App)

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "state": "stopped"})
}

func (h *InstancesHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	h.log.Event("INSTANCE RESTARTING", "old_id", id, "app", inst.App, "version", inst.Version)

	h.stopContainer(id, inst.ContainerID, inst.App)

	newInst := h.store.CreateInstance(inst.App, inst.Version, inst.Image)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": newInst.ID,
		"app":         inst.App,
		"version":     inst.Version,
	})

	h.runContainer(newInst.ID, newInst.ContainerID, inst.Image, inst.App, inst.Version)
	newInst, _ = h.store.GetInstance(newInst.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"old_id":     id,
		"new_id":     newInst.ID,
		"app":        inst.App,
		"version":    inst.Version,
		"state":      newInst.State,
		"started_at": newInst.StartedAt,
	})
}

func (h *InstancesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	latest := h.registry.LatestVersion(inst.App)
	if latest == "" {
		writeError(w, http.StatusNotFound, "could not determine latest version")
		return
	}
	if inst.Version == latest {
		writeJSON(w, http.StatusOK, map[string]any{
			"message": fmt.Sprintf("already running latest version %s", latest),
		})
		return
	}

	imageRef := h.registry.ImageRef(inst.App, latest)
	h.log.Event("INSTANCE UPDATING", "id", id, "app", inst.App, "from", inst.Version, "to", latest)

	h.stopContainer(id, inst.ContainerID, inst.App)

	newInst := h.store.CreateInstance(inst.App, latest, imageRef)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": newInst.ID,
		"app":         inst.App,
		"version":     latest,
	})

	h.runContainer(newInst.ID, newInst.ContainerID, imageRef, inst.App, latest)
	newInst, _ = h.store.GetInstance(newInst.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"old_id":      id,
		"old_version": inst.Version,
		"new_id":      newInst.ID,
		"new_version": latest,
		"state":       newInst.State,
		"started_at":  newInst.StartedAt,
	})
}

func (h *InstancesHandler) Logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	tail := "100"
	if t := r.URL.Query().Get("tail"); t != "" {
		tail = t
	}
	since := r.URL.Query().Get("since")

	// --- Non-streaming: plain text log dump ---
	if r.URL.Query().Get("stream") != "true" {
		args := []string{"logs", "--tail", tail}
		if since != "" {
			args = append(args, "--since", since)
		}
		args = append(args, inst.ContainerID)
		out, err := containerExec(args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch logs: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(out))
		return
	}

	// --- SSE streaming via container logs --follow ---
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Support reconnection via Last-Event-ID header (SSE spec).
	if since == "" {
		if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
			since = lastID
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Build container CLI command with separate stdout/stderr and timestamps.
	args := []string{"logs", "--follow", "--timestamps", "--tail", tail}
	if since != "" {
		args = append(args, "--since", since)
	}
	args = append(args, inst.ContainerID)

	cmd := exec.CommandContext(r.Context(), ContainerCLI(), args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		writeSSE(w, flusher, "", "error", marshalJSON(map[string]string{
			"ts": nowUTC(), "message": "failed to create stdout pipe: " + err.Error(),
		}))
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		writeSSE(w, flusher, "", "error", marshalJSON(map[string]string{
			"ts": nowUTC(), "message": "failed to create stderr pipe: " + err.Error(),
		}))
		return
	}

	if err := cmd.Start(); err != nil {
		writeSSE(w, flusher, "", "error", marshalJSON(map[string]string{
			"ts": nowUTC(), "message": "failed to start log process: " + err.Error(),
		}))
		return
	}
	defer cmd.Wait()

	// Merge stdout, stderr, and heartbeat into a single channel.
	lines := make(chan taggedLine, 64)
	done := make(chan struct{})
	defer close(done)

	var wg sync.WaitGroup
	wg.Add(2)

	// Scanner goroutine for a pipe — tags each line with the stream name.
	scanPipe := func(pipe io.Reader, stream string) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			select {
			case lines <- taggedLine{stream: stream, line: scanner.Text()}:
			case <-done:
				return
			}
		}
	}

	go scanPipe(stdoutPipe, "stdout")
	go scanPipe(stderrPipe, "stderr")

	// Close the lines channel once both scanners finish.
	go func() {
		wg.Wait()
		close(lines)
	}()

	// Heartbeat goroutine — sends a sentinel every 15 seconds.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case lines <- taggedLine{stream: "heartbeat"}:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Main loop: read merged channel, write SSE events.
	var seq int64
	for tl := range lines {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		if tl.stream == "heartbeat" {
			writeSSE(w, flusher, "", "heartbeat", marshalJSON(map[string]string{
				"ts": nowUTC(),
			}))
			continue
		}

		seq++
		ts, content, _ := parseNerdctlTimestamp(tl.line)

		evt := logEvent{
			Timestamp: ts,
			Seq:       seq,
			Stream:    tl.stream,
			Content:   content,
		}
		writeSSE(w, flusher, strconv.FormatInt(seq, 10), "log", marshalJSON(evt))
	}

	// Both pipes closed — stream has ended.
	writeSSE(w, flusher, "", "error", marshalJSON(map[string]string{
		"ts": nowUTC(), "message": "stream ended",
	}))
}

func (h *InstancesHandler) Health(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	out, err := containerExec("inspect", "--format", "{{.State.Running}}", inst.ContainerID)
	healthy := err == nil && strings.TrimSpace(out) == "true"
	output := "OK"
	if !healthy {
		output = fmt.Sprintf("instance is %s", inst.State)
		if err != nil {
			output = err.Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"healthy":      healthy,
		"last_checked": time.Now().UTC(),
		"output":       output,
	})
}

func (h *InstancesHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	uptime := int64(0)
	if inst.StoppedAt == nil {
		uptime = int64(time.Since(inst.StartedAt).Seconds())
	}

	out, err := containerExec("stats", "--no-stream", "--format", "{{json .}}", inst.ContainerID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":             id,
			"uptime_seconds": uptime,
			"error":          err.Error(),
		})
		return
	}

	cpu, mem := parseStats(out)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             id,
		"cpu_percent":    cpu,
		"memory_mb":      mem,
		"uptime_seconds": uptime,
	})
}

// --- State reconciliation ---

// reconcileInstanceState checks the actual Docker daemon to see if a running instance
// has exited, and updates its stored state if needed. This prevents stale state where
// the instance shows "running" even after the container has exited.
func (h *InstancesHandler) reconcileInstanceState(inst *store.Instance) {
	// Only reconcile instances that we think are running or starting.
	if inst.State != "running" && inst.State != "starting" {
		return
	}

	// Check actual container state via container inspect.
	out, err := containerExec("inspect", "--format", "{{.State.Running}}", inst.ContainerID)
	if err != nil {
		// Container doesn't exist or inspect failed — mark as crashed.
		h.store.SetInstanceState(inst.ID, "crashed", nil)
		h.store.SetInstanceError(inst.ID, "container exited: "+err.Error())
		inst.State = "crashed"
		inst.Error = "container exited: " + err.Error()
		return
	}

	isRunning := strings.TrimSpace(out) == "true"
	if !isRunning {
		// Container exists but is not running — mark as stopped.
		exitCode := 0
		h.store.SetInstanceState(inst.ID, "stopped", &exitCode)
		inst.State = "stopped"
		inst.ExitCode = &exitCode
	}
}

// --- container helpers ---

func (h *InstancesHandler) runContainer(instanceID, containerID, image, app, version string) {
	if _, err := containerExec("run", "-d", "--name", containerID, "--network", "host", image); err != nil {
		h.log.Error("INSTANCE START FAILED", "id", instanceID, "image", image, "err", err)
		exitCode := 1
		h.store.SetInstanceState(instanceID, "crashed", &exitCode)
		h.store.SetInstanceError(instanceID, err.Error())
		h.store.AddEvent("instance_start_failed", map[string]any{
			"instance_id": instanceID,
			"app":         app,
			"version":     version,
			"error":       err.Error(),
		})
		return
	}

	h.store.SetInstanceState(instanceID, "running", nil)
	h.log.Event("INSTANCE RUNNING", "app", app, "version", version, "id", instanceID)
}

func (h *InstancesHandler) stopContainer(instanceID, containerID, app string) {
	containerExec("stop", containerID)
	containerExec("rm", containerID)

	exitCode := 0
	h.store.SetInstanceState(instanceID, "stopped", &exitCode)
	h.log.Event("INSTANCE STOPPED", "id", instanceID, "app", app, "exit_code", 0)
	h.store.AddEvent("instance_stopped", map[string]any{
		"instance_id": instanceID,
		"app":         app,
		"exit_code":   0,
	})
}

// StartReconciler runs a background loop that periodically reconciles
// all "running"/"starting" instances with the actual container runtime state.
// This ensures Cortex never reports a stale "running" state for a container
// that has exited between API calls.
func (h *InstancesHandler) StartReconciler(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	h.log.Info("instance reconciler started", "interval", "10s", "cli", ContainerCLI())
	for {
		select {
		case <-ctx.Done():
			h.log.Info("instance reconciler stopped")
			return
		case <-ticker.C:
			h.reconcileAll()
		}
	}
}

// reconcileAll checks every "running" or "starting" instance against the
// actual container runtime and corrects any stale state.
func (h *InstancesHandler) reconcileAll() {
	instances := h.store.ListInstances()
	for _, inst := range instances {
		if inst.State != "running" && inst.State != "starting" {
			continue
		}
		before := inst.State
		h.reconcileInstanceState(inst)
		if inst.State != before {
			h.log.Event("RECONCILED", "id", inst.ID, "app", inst.App,
				"from", before, "to", inst.State)
			h.store.AddEvent("instance_reconciled", map[string]any{
				"instance_id": inst.ID,
				"app":         inst.App,
				"old_state":   before,
				"new_state":   inst.State,
			})
		}
	}
}

// parseStats pulls cpu percent and memory MB from nerdctl stats JSON output.
// Example: {"CPUPerc":"0.42%","MemUsage":"84.5MiB / 1.796GiB",...}
func parseStats(raw string) (cpuPercent float64, memMB float64) {
	var s struct {
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return 0, 0
	}

	cpuStr := strings.TrimSuffix(s.CPUPerc, "%")
	cpuPercent, _ = strconv.ParseFloat(cpuStr, 64)

	// "84.5MiB / 1.796GiB" — take the left side
	memStr := strings.Split(s.MemUsage, " / ")[0]
	memMB = parseMemToMB(memStr)
	return
}

func parseMemToMB(s string) float64 {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		factor float64
	}{
		{"GiB", 1024},
		{"MiB", 1},
		{"KiB", 1.0 / 1024},
		{"GB", 953.674},
		{"MB", 0.953674},
		{"kB", 0.000953674},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err == nil {
				return v * u.factor
			}
		}
	}
	return 0
}

// --- SSE helpers ---

// writeSSE writes a single Server-Sent Events frame and flushes.
func writeSSE(w io.Writer, flusher http.Flusher, id string, event string, data []byte) {
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// parseNerdctlTimestamp splits a nerdctl --timestamps log line into the
// RFC3339Nano timestamp and the remaining content. If parsing fails the
// full line is returned as content with the current time.
func parseNerdctlTimestamp(line string) (ts string, content string, ok bool) {
	if idx := strings.IndexByte(line, ' '); idx > 0 {
		if t, err := time.Parse(time.RFC3339Nano, line[:idx]); err == nil {
			return t.UTC().Format(time.RFC3339Nano), line[idx+1:], true
		}
	}
	return nowUTC(), line, false
}

// marshalJSON serialises v to JSON, returning "{}" on error.
func marshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// nowUTC returns the current time as an RFC3339Nano string.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// --- response helpers ---

func instanceSummary(inst *store.Instance) map[string]any {
	return map[string]any{
		"id":           inst.ID,
		"app":          inst.App,
		"version":      inst.Version,
		"image":        inst.Image,
		"state":        inst.State,
		"error":        inst.Error,
		"started_at":   inst.StartedAt,
		"stopped_at":   inst.StoppedAt,
	}
}

func instanceFull(inst *store.Instance) map[string]any {
	m := instanceSummary(inst)
	m["container_id"] = inst.ContainerID
	m["exit_code"] = inst.ExitCode
	return m
}
