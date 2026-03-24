package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"cortex/internal/logger"
	"cortex/internal/registry"
	"cortex/internal/store"
)

type InstancesHandler struct {
	store    *store.Store
	registry *registry.Registry
	log      *logger.Logger
}

func NewInstancesHandler(s *store.Store, r *registry.Registry, l *logger.Logger) *InstancesHandler {
	return &InstancesHandler{store: s, registry: r, log: l}
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

	inst, _ = h.store.GetInstance(inst.ID)
	writeJSON(w, http.StatusCreated, instanceSummary(inst))
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

	if r.URL.Query().Get("stream") != "true" {
		out, err := nerdctl("logs", "--tail", tail, inst.ContainerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch logs: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(out))
		return
	}

	// SSE streaming via nerdctl logs --follow.
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	cmd := exec.CommandContext(r.Context(), "nerdctl", "logs", "--follow", "--tail", tail, inst.ContainerID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
			fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			flusher.Flush()
		}
	}
}

func (h *InstancesHandler) Health(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	out, err := nerdctl("inspect", "--format", "{{.State.Running}}", inst.ContainerID)
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

	out, err := nerdctl("stats", "--no-stream", "--format", "{{json .}}", inst.ContainerID)
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

// --- nerdctl helpers ---

func (h *InstancesHandler) runContainer(instanceID, containerID, image, app, version string) {
	if _, err := nerdctl("run", "-d", "--name", containerID, "--network", "host", image); err != nil {
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
	nerdctl("stop", containerID)
	nerdctl("rm", containerID)

	exitCode := 0
	h.store.SetInstanceState(instanceID, "stopped", &exitCode)
	h.log.Event("INSTANCE STOPPED", "id", instanceID, "app", app, "exit_code", 0)
	h.store.AddEvent("instance_stopped", map[string]any{
		"instance_id": instanceID,
		"app":         app,
		"exit_code":   0,
	})
}

// nerdctl runs a nerdctl command and returns combined stdout+stderr output.
func nerdctl(args ...string) (string, error) {
	out, err := exec.Command("nerdctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
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
