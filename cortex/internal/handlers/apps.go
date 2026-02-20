package handlers

import (
	"net/http"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type AppsHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewAppsHandler(s *store.Store, l *logger.Logger) *AppsHandler {
	return &AppsHandler{store: s, log: l}
}

func (h *AppsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /apps", h.List)
	mux.HandleFunc("GET /apps/{name}/versions", h.Versions)
}

func (h *AppsHandler) List(w http.ResponseWriter, r *http.Request) {
	apps := h.store.ListApps()
	out := make([]map[string]any, 0, len(apps))
	for _, a := range apps {
		out = append(out, map[string]any{
			"name":           a.Name,
			"repo":           a.Repo,
			"description":    a.Description,
			"latest_version": a.LatestVersion,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

func (h *AppsHandler) Versions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := h.store.GetApp(name); !ok {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	versions, _ := h.store.GetVersions(name)
	out := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		out = append(out, map[string]any{
			"version":      v.Version,
			"published_at": v.PublishedAt,
			"changelog":    v.Changelog,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": name, "versions": out})
}
