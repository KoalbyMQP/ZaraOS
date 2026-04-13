package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"cortex/internal/logger"
	"cortex/internal/registry"
	"cortex/internal/requirements"
	"cortex/internal/store"
)

// RequirementsHandler exposes the package requirements system via HTTP.
type RequirementsHandler struct {
	bootloader *requirements.Bootloader
	store      *store.Store
	registry   *registry.Registry
	log        *logger.Logger
}

func NewRequirementsHandler(
	bl *requirements.Bootloader,
	s *store.Store,
	r *registry.Registry,
	l *logger.Logger,
) *RequirementsHandler {
	return &RequirementsHandler{bootloader: bl, store: s, registry: r, log: l}
}

func (h *RequirementsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /requirements", h.GetManifest)
	mux.HandleFunc("PUT /requirements", h.PutManifest)
	mux.HandleFunc("GET /requirements/packages", h.ListPackages)
	mux.HandleFunc("GET /requirements/packages/{name}", h.GetPackage)
	mux.HandleFunc("PUT /requirements/packages/{name}", h.UpdatePackage)
	mux.HandleFunc("DELETE /requirements/packages/{name}", h.DeletePackage)
	mux.HandleFunc("GET /requirements/updates", h.CheckUpdates)
	mux.HandleFunc("POST /requirements/updates/{name}", h.ApplyUpdate)
	mux.HandleFunc("POST /requirements/discover", h.Discover)
	mux.HandleFunc("POST /requirements/boot", h.TriggerBoot)
}

// GetManifest returns the current requirements manifest.
func (h *RequirementsHandler) GetManifest(w http.ResponseWriter, r *http.Request) {
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// PutManifest replaces the entire manifest, validates it, and saves to disk.
func (h *RequirementsHandler) PutManifest(w http.ResponseWriter, r *http.Request) {
	var m requirements.Manifest
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := m.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	path := h.bootloader.ManifestPath()
	if err := m.Save(path); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save manifest: "+err.Error())
		return
	}

	h.log.Event("MANIFEST UPDATED", "packages", len(m.Packages))
	h.store.AddEvent("manifest_updated", map[string]any{
		"packages": len(m.Packages),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "manifest updated",
		"packages": len(m.Packages),
	})
}

// ListPackages returns all packages with their current runtime status.
func (h *RequirementsHandler) ListPackages(w http.ResponseWriter, r *http.Request) {
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}

	instances := h.store.ListInstances()
	instByApp := make(map[string]*store.Instance)
	for _, inst := range instances {
		// Keep the most recent (or running) instance for each app.
		existing, ok := instByApp[inst.App]
		if !ok || inst.State == "running" || inst.State == "starting" {
			cp := inst
			instByApp[inst.App] = &cp
		}
	}

	out := make([]map[string]any, 0, len(m.Packages))
	for _, pkg := range m.Packages {
		status := map[string]any{
			"name":          pkg.Name,
			"image":         pkg.Image,
			"version":       pkg.Version,
			"essential":     pkg.Essential,
			"autostart":     pkg.Autostart,
			"priority":      pkg.Priority,
			"depends":       pkg.Depends,
			"run_condition": pkg.RunCondition,
			"managed":       h.store.IsPackageManaged(pkg.Name),
		}

		if inst, ok := instByApp[pkg.Name]; ok {
			status["state"] = inst.State
			status["instance_id"] = inst.ID
			status["running_version"] = inst.Version
			status["error"] = inst.Error
			status["started_at"] = inst.StartedAt
		} else {
			status["state"] = "not_running"
		}

		// Check condition.
		status["condition_met"] = requirements.EvalRunCondition(pkg.RunCondition)

		out = append(out, status)
	}

	writeJSON(w, http.StatusOK, map[string]any{"packages": out})
}

// GetPackage returns a single package's status by name.
func (h *RequirementsHandler) GetPackage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}

	pkg, ok := m.GetPackage(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("package %q not found", name))
		return
	}

	status := map[string]any{
		"name":          pkg.Name,
		"image":         pkg.Image,
		"version":       pkg.Version,
		"essential":     pkg.Essential,
		"autostart":     pkg.Autostart,
		"priority":      pkg.Priority,
		"depends":       pkg.Depends,
		"run_condition": pkg.RunCondition,
		"condition_met": requirements.EvalRunCondition(pkg.RunCondition),
		"managed":       h.store.IsPackageManaged(pkg.Name),
	}

	// Find running instance.
	for _, inst := range h.store.ListInstances() {
		if inst.App == pkg.Name && (inst.State == "running" || inst.State == "starting") {
			status["state"] = inst.State
			status["instance_id"] = inst.ID
			status["running_version"] = inst.Version
			status["started_at"] = inst.StartedAt
			break
		}
	}
	if _, ok := status["state"]; !ok {
		status["state"] = "not_running"
	}

	// Check for available update.
	latest := h.registry.LatestVersion(pkg.Name)
	if latest != "" {
		status["latest_version"] = latest
	}

	writeJSON(w, http.StatusOK, status)
}

// UpdatePackage modifies a single package's configuration in the manifest.
func (h *RequirementsHandler) UpdatePackage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}

	// Parse partial update.
	var update struct {
		Version   *string `json:"version,omitempty"`
		Autostart *bool   `json:"autostart,omitempty"`
		Essential *bool   `json:"essential,omitempty"`
		Priority  *int    `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Find and modify the package.
	found := false
	for i := range m.Packages {
		if m.Packages[i].Name != name {
			continue
		}
		found = true
		if update.Version != nil {
			m.Packages[i].Version = *update.Version
		}
		if update.Autostart != nil {
			m.Packages[i].Autostart = *update.Autostart
		}
		if update.Essential != nil {
			m.Packages[i].Essential = *update.Essential
		}
		if update.Priority != nil {
			m.Packages[i].Priority = *update.Priority
		}
		break
	}

	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("package %q not found", name))
		return
	}

	// Validate and save.
	if err := m.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed after update: "+err.Error())
		return
	}
	if err := m.Save(h.bootloader.ManifestPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save: "+err.Error())
		return
	}

	h.log.Event("PACKAGE UPDATED", "package", name)
	writeJSON(w, http.StatusOK, map[string]any{"updated": name})
}

// DeletePackage removes a package from the manifest and stops its container.
// Refuses to delete essential packages and packages that others depend on.
func (h *RequirementsHandler) DeletePackage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}

	pkg, ok := m.GetPackage(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("package %q not found", name))
		return
	}

	// Guard: never delete essential packages.
	if pkg.Essential {
		writeError(w, http.StatusForbidden,
			fmt.Sprintf("cannot delete essential package %q — this would brick the robot", name))
		return
	}

	// Guard: check if any other package depends on this one.
	for _, other := range m.Packages {
		if other.Name == name {
			continue
		}
		for _, dep := range other.Depends {
			if dep == name {
				writeError(w, http.StatusConflict,
					fmt.Sprintf("cannot delete %q — package %q depends on it", name, other.Name))
				return
			}
		}
	}

	// Remove from manifest.
	if err := m.RemovePackage(name); err != nil {
		writeError(w, http.StatusInternalServerError, "remove failed: "+err.Error())
		return
	}
	if err := m.Save(h.bootloader.ManifestPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save: "+err.Error())
		return
	}

	// Stop and clean up any running containers for this package.
	for _, inst := range h.store.ListInstances() {
		if inst.App == name && (inst.State == "running" || inst.State == "starting") {
			exec.Command(ContainerCLI(), "stop", inst.ContainerID).Run()
			exec.Command(ContainerCLI(), "rm", inst.ContainerID).Run()
			exitCode := 0
			h.store.SetInstanceState(inst.ID, "stopped", &exitCode)
		}
	}
	h.store.SetPackageManaged(name, false)

	h.log.Event("PACKAGE DELETED", "package", name)
	h.store.AddEvent("package_deleted", map[string]any{"package": name})

	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

// CheckUpdates returns version comparison for all packages.
func (h *RequirementsHandler) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	updates := h.bootloader.CheckUpdates()
	writeJSON(w, http.StatusOK, map[string]any{"updates": updates})
}

// ApplyUpdate pulls the latest version and restarts a specific package.
func (h *RequirementsHandler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusNotFound, "no manifest loaded")
		return
	}

	pkg, ok := m.GetPackage(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("package %q not found", name))
		return
	}

	latest := h.registry.LatestVersion(pkg.Name)
	if latest == "" {
		writeError(w, http.StatusNotFound, "no versions available in registry")
		return
	}

	imageRef := pkg.ImageRef(latest)
	h.log.Info("requirements: applying update",
		"package", name,
		"version", latest,
		"image", imageRef,
	)

	// Pull the new image.
	out, err := exec.Command(ContainerCLI(), "pull", imageRef).CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("pull failed: %s — %s", strings.TrimSpace(string(out)), err.Error()))
		return
	}

	// Stop running instance(s) of this package.
	oldVersion := ""
	for _, inst := range h.store.ListInstances() {
		if inst.App == name && (inst.State == "running" || inst.State == "starting") {
			oldVersion = inst.Version
			exec.Command(ContainerCLI(), "stop", inst.ContainerID).Run()
			exec.Command(ContainerCLI(), "rm", inst.ContainerID).Run()
			exitCode := 0
			h.store.SetInstanceState(inst.ID, "stopped", &exitCode)
		}
	}

	// Start new instance.
	inst := h.store.CreateInstance(name, latest, imageRef)
	args := []string{"run", "-d", "--name", inst.ContainerID, "--network", "host"}
	for k, v := range pkg.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, imageRef)

	out, err = exec.Command(ContainerCLI(), args...).CombinedOutput()
	if err != nil {
		exitCode := 1
		h.store.SetInstanceState(inst.ID, "crashed", &exitCode)
		h.store.SetInstanceError(inst.ID, err.Error())
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("start failed: %s — %s", strings.TrimSpace(string(out)), err.Error()))
		return
	}

	h.store.SetInstanceState(inst.ID, "running", nil)

	h.log.Event("PACKAGE UPDATED", "package", name, "from", oldVersion, "to", latest)
	h.store.AddEvent("requirement_updated", map[string]any{
		"package":     name,
		"old_version": oldVersion,
		"new_version": latest,
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"name":        name,
		"old_version": oldVersion,
		"new_version": latest,
		"instance_id": inst.ID,
		"state":       "running",
	})
}

// Discover pulls a container image and reads its embedded zaraos.json,
// then merges the package into the active manifest. This is how new
// packages get added — each repo ships its own zaraos.json, and Cortex
// discovers it automatically.
//
//	POST /requirements/discover  {"image": "koalby/motor-driver:v1.0.0"}
func (h *RequirementsHandler) Discover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image   string `json:"image"`
		Version string `json:"version"` // optional, defaults to tag from image ref
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Image == "" {
		writeError(w, http.StatusBadRequest, "missing required field: image")
		return
	}

	m := h.bootloader.Manifest()
	if m == nil {
		writeError(w, http.StatusInternalServerError, "no manifest loaded")
		return
	}

	h.log.Info("requirements: discovering package", "image", req.Image)

	// Pull the image first.
	out, err := exec.Command(ContainerCLI(), "pull", req.Image).CombinedOutput()
	if err != nil {
		writeError(w, http.StatusBadGateway,
			fmt.Sprintf("pull failed: %s — %s", strings.TrimSpace(string(out)), err.Error()))
		return
	}

	// Read the embedded zaraos.json from the image.
	meta, err := requirements.ReadPackageMeta(req.Image)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("no zaraos.json in image: %s", err.Error()))
		return
	}

	// Determine version.
	version := req.Version
	if version == "" {
		// Try to extract tag from image ref (e.g. "koalby/motor-driver:v1.0.0" -> "v1.0.0")
		if parts := strings.SplitN(req.Image, ":", 2); len(parts) == 2 {
			version = parts[1]
		} else {
			version = "latest"
		}
	}

	// Merge into manifest.
	pkg := meta.ToPackage(version)
	if err := m.MergePackage(pkg); err != nil {
		writeError(w, http.StatusBadRequest, "merge failed: "+err.Error())
		return
	}

	// Save updated manifest.
	if err := m.Save(h.bootloader.ManifestPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save manifest: "+err.Error())
		return
	}

	h.log.Event("PACKAGE DISCOVERED",
		"package", meta.Name,
		"image", req.Image,
		"version", version,
		"depends", meta.Depends,
	)
	h.store.AddEvent("package_discovered", map[string]any{
		"package": meta.Name,
		"image":   req.Image,
		"version": version,
		"depends": meta.Depends,
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"package":  meta.Name,
		"image":    meta.Image,
		"version":  version,
		"depends":  meta.Depends,
		"priority": meta.Priority,
		"merged":   true,
	})
}

// TriggerBoot re-runs the boot sequence (reloads manifest and starts packages).
func (h *RequirementsHandler) TriggerBoot(w http.ResponseWriter, r *http.Request) {
	h.log.Event("BOOT SEQUENCE TRIGGERED")
	go func() {
		if err := h.bootloader.Run(r.Context()); err != nil {
			h.log.Error("requirements: triggered boot failed", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "boot sequence started",
	})
}
