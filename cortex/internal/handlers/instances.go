package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type InstancesHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewInstancesHandler(s *store.Store, l *logger.Logger) *InstancesHandler {
	return &InstancesHandler{store: s, log: l}
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.App == "" {
		writeError(w, http.StatusBadRequest, "missing required field: app")
		return
	}
	if req.Version == "" {
		req.Version = "latest"
	}

	app, ok := h.store.GetApp(req.App)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %s not found", req.App))
		return
	}

	version, ok := h.store.ResolveVersion(req.App, req.Version)
	if !ok {
		writeError(w, http.StatusNotFound,
			fmt.Sprintf("version %s not found for app %s", req.Version, req.App))
		return
	}

	// ros2-nav is treated as a singleton for demo purposes.
	if app.Name == "ros2-nav" && h.store.HasRunningInstance(req.App) {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("an instance of %s is already running", req.App))
		return
	}

	inst := h.store.CreateInstance(req.App, version)
	h.log.Event("INSTANCE STARTING", "app", req.App, "version", version, "id", inst.ID)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": inst.ID,
		"app":         req.App,
		"version":     version,
	})

	go func() {
		time.Sleep(600 * time.Millisecond)
		h.store.SetInstanceState(inst.ID, "running", nil)
		h.log.Event("INSTANCE RUNNING", "app", req.App, "version", version, "id", inst.ID)
	}()

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
	h.log.Event("INSTANCE STOPPING", "id", id, "app", inst.App, "hint", "SIGTERM sent, waiting up to 10s")

	go func() {
		time.Sleep(700 * time.Millisecond)
		exitCode := 0
		h.store.SetInstanceState(id, "stopped", &exitCode)
		h.log.Event("INSTANCE STOPPED", "id", id, "app", inst.App, "exit_code", 0)
		h.store.AddEvent("instance_stopped", map[string]any{
			"instance_id": id,
			"app":         inst.App,
			"exit_code":   0,
		})
	}()

	exitCode := 0
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        id,
		"state":     "stopping",
		"exit_code": exitCode,
	})
}

func (h *InstancesHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	exitCode := 0
	h.store.SetInstanceState(id, "stopped", &exitCode)
	h.log.Event("INSTANCE RESTARTING", "old_id", id, "app", inst.App, "version", inst.Version)

	newInst := h.store.CreateInstance(inst.App, inst.Version)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": newInst.ID,
		"app":         inst.App,
		"version":     inst.Version,
	})

	go func() {
		time.Sleep(600 * time.Millisecond)
		h.store.SetInstanceState(newInst.ID, "running", nil)
		h.log.Event("INSTANCE RUNNING", "app", inst.App, "version", inst.Version, "id", newInst.ID)
	}()

	writeJSON(w, http.StatusCreated, map[string]any{
		"old_id":     id,
		"new_id":     newInst.ID,
		"app":        inst.App,
		"version":    inst.Version,
		"state":      "starting",
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

	app, _ := h.store.GetApp(inst.App)
	latest := app.LatestVersion

	if inst.Version == latest {
		writeJSON(w, http.StatusOK, map[string]any{
			"error": fmt.Sprintf("already running latest version %s", latest),
		})
		return
	}

	exitCode := 0
	h.store.SetInstanceState(id, "stopped", &exitCode)
	h.log.Event("INSTANCE UPDATING", "id", id, "app", inst.App, "from", inst.Version, "to", latest)

	newInst := h.store.CreateInstance(inst.App, latest)
	h.store.AddEvent("instance_started", map[string]any{
		"instance_id": newInst.ID,
		"app":         inst.App,
		"version":     latest,
	})

	go func() {
		time.Sleep(600 * time.Millisecond)
		h.store.SetInstanceState(newInst.ID, "running", nil)
		h.log.Event("INSTANCE RUNNING", "app", inst.App, "version", latest, "id", newInst.ID)
	}()

	writeJSON(w, http.StatusCreated, map[string]any{
		"old_id":      id,
		"old_version": inst.Version,
		"new_id":      newInst.ID,
		"new_version": latest,
		"state":       "starting",
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

	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			tail = n
		}
	}

	fakeLogs := generateFakeLogs(inst, tail)

	if r.URL.Query().Get("stream") != "true" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, strings.Join(fakeLogs, "\n")+"\n")
		return
	}

	// SSE stream
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, line := range fakeLogs {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	flusher.Flush()

	tick := time.NewTicker(3 * time.Second)
	ka := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	defer ka.Stop()

	lineN := len(fakeLogs)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ka.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case t := <-tick.C:
			lineN++
			line := fmt.Sprintf("%s [INFO] %s", t.UTC().Format(time.RFC3339), streamLine(inst.App, lineN))
			fmt.Fprintf(w, "data: %s\n\n", line)
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
	healthy := inst.State == "running"
	output := "OK"
	if !healthy {
		output = fmt.Sprintf("instance is %s", inst.State)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             id,
		"cpu_percent":    12.4,
		"memory_mb":      84,
		"uptime_seconds": uptime,
	})
}

// --- helpers ---

func instanceSummary(inst *store.Instance) map[string]any {
	return map[string]any{
		"id":         inst.ID,
		"app":        inst.App,
		"version":    inst.Version,
		"state":      inst.State,
		"started_at": inst.StartedAt,
		"stopped_at": inst.StoppedAt,
	}
}

func instanceFull(inst *store.Instance) map[string]any {
	m := instanceSummary(inst)
	m["pid"] = inst.PID
	m["exit_code"] = inst.ExitCode
	return m
}

var bootLogs = map[string][]string{
	"ros2-nav": {
		"Navigation node started",
		"LIDAR connected on /dev/ttyUSB0",
		"Waiting for map...",
		"Map received from /map topic",
		"Navigation ready",
		"Goal received: (3.2, 1.5)",
		"Path planned: 12 waypoints",
		"Obstacle detected at (2.1, 0.8) — replanning",
		"New path: 14 waypoints",
		"Goal reached in 8.3s",
	},
	"camera-driver": {
		"Camera driver started",
		"Device found at /dev/video0",
		"Initializing H264 encoder",
		"Streaming started at 1920x1080 30fps",
		"Streaming OK — 2.1 Mbps",
	},
	"lidar-proc": {
		"LIDAR processor started",
		"Subscribed to /scan topic",
		"Voxel grid filter initialized: leaf_size=0.05",
		"Processing 50000 points/sec",
		"Published /processed_scan",
	},
}

var streamLogs = map[string][]string{
	"ros2-nav":      {"Heartbeat OK", "Position: (1.2, 0.5)", "Velocity: 0.3 m/s", "Map updated", "Battery: 78%"},
	"camera-driver": {"Frame captured", "Encode OK", "Buffer healthy", "Streaming OK"},
	"lidar-proc":    {"Scan processed", "75230 pts", "Published /processed_scan"},
}

func generateFakeLogs(inst *store.Instance, tail int) []string {
	msgs := bootLogs[inst.App]
	if msgs == nil {
		msgs = []string{"Process started", "Running..."}
	}
	var lines []string
	t := inst.StartedAt
	for i, msg := range msgs {
		if i >= tail {
			break
		}
		t = t.Add(time.Duration(i+1) * time.Second)
		lines = append(lines, fmt.Sprintf("%s [INFO] %s", t.UTC().Format(time.RFC3339), msg))
	}
	return lines
}

func streamLine(app string, n int) string {
	msgs := streamLogs[app]
	if msgs == nil {
		msgs = []string{"heartbeat"}
	}
	return msgs[n%len(msgs)]
}
