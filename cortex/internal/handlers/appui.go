package handlers

// App UI: what the Driver Station needs to show an app's own pages.
//
//	GET /ui/{app}/{version}/{path...}   static UI bundle from the app's image (public)
//
// Instances whose image ships a bundle carry "ui": {"descriptor_url": ...}.

import (
	"bytes"
	"net/http"

	"cortex/internal/appui"
	"cortex/internal/logger"
	"cortex/internal/store"
)

type AppUIHandler struct {
	ui  *appui.Store
	log *logger.Logger
}

func NewAppUIHandler(ui *appui.Store, l *logger.Logger) *AppUIHandler {
	return &AppUIHandler{ui: ui, log: l}
}

// RegisterPublic mounts the bundle files without auth. A browser loads them
// as scripts and stylesheets, which cannot carry signing headers.
func (h *AppUIHandler) RegisterPublic(mux *http.ServeMux) {
	mux.HandleFunc("GET /ui/{app}/{version}/{path...}", h.Asset)
}

func (h *AppUIHandler) Asset(w http.ResponseWriter, r *http.Request) {
	bundle, ok := h.ui.Bundle(r.PathValue("app"), r.PathValue("version"))
	if !ok {
		writeError(w, http.StatusNotFound, "app has no UI")
		return
	}
	name := r.PathValue("path")
	file, ok := bundle.File(name)
	if !ok {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// A rebuilt image keeps its app name and tag, so the same URL can change
	// content. Make browsers revalidate instead of trusting their cache.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", file.ETag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	http.ServeContent(w, r, name, bundle.LoadedAt, bytes.NewReader(file.Data))
}

// SetAppUI wires the UI bundle store so instances advertise their UI.
func (h *InstancesHandler) SetAppUI(ui *appui.Store) {
	h.ui = ui
}

// withUI adds the "ui" field to an instance response when its image ships a
// UI bundle, and lets the store examine instances it has not seen yet.
func (h *InstancesHandler) withUI(inst *store.Instance, m map[string]any) map[string]any {
	if h.ui == nil {
		return m
	}
	h.ui.Observe(inst.ID, inst.App, inst.Version, inst.Image, inst.State)
	if u, ok := h.ui.DescriptorURL(inst.App, inst.Version); ok {
		m["ui"] = map[string]any{"descriptor_url": u}
	}
	return m
}
