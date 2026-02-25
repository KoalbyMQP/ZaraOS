package handlers

import (
	"net/http"

	"cortex/internal/logger"
	"cortex/internal/registry"
)

type AppsHandler struct {
	registry *registry.Registry
	log      *logger.Logger
}

func NewAppsHandler(r *registry.Registry, l *logger.Logger) *AppsHandler {
	return &AppsHandler{registry: r, log: l}
}

func (h *AppsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /apps", h.List)
	mux.HandleFunc("GET /apps/{name}/versions", h.Versions)
}

func (h *AppsHandler) List(w http.ResponseWriter, r *http.Request) {
	apps := h.registry.ListApps()
	out := make([]map[string]any, 0, len(apps))
	for _, a := range apps {
		latest := ""
		if len(a.Versions) > 0 {
			latest = a.Versions[0].Version
		}
		out = append(out, map[string]any{
			"name":           a.Name,
			"repo":           a.Repo,
			"latest_version": latest,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

func (h *AppsHandler) Versions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	app, ok := h.registry.GetApp(name)
	if !ok {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	out := make([]map[string]any, 0, len(app.Versions))
	for _, v := range app.Versions {
		out = append(out, map[string]any{
			"version":      v.Version,
			"published_at": v.PublishedAt,
			"changelog":    v.Changelog,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": name, "versions": out})
}
