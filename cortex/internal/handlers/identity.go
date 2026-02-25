package handlers

import (
	"encoding/json"
	"net/http"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type IdentityHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewIdentityHandler(s *store.Store, l *logger.Logger) *IdentityHandler {
	return &IdentityHandler{store: s, log: l}
}

func (h *IdentityHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /identity", h.Get)
	mux.HandleFunc("PUT /identity/location", h.SetLocation)
	mux.HandleFunc("GET /ssh", h.SSH)
}

func (h *IdentityHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := h.store.GetIdentity()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":             id.Name,
		"serial":           id.Serial,
		"location":         id.Location,
		"firmware_version": id.FirmwareVersion,
	})
}

func (h *IdentityHandler) SetLocation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Location string `json:"location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Location == "" {
		writeError(w, http.StatusBadRequest, "missing required field: location")
		return
	}
	h.store.SetLocation(req.Location)
	h.log.Event("LOCATION UPDATED", "location", req.Location)
	writeJSON(w, http.StatusOK, map[string]any{"location": req.Location})
}

func (h *IdentityHandler) SSH(w http.ResponseWriter, r *http.Request) {
	id := h.store.GetIdentity()
	writeJSON(w, http.StatusOK, map[string]any{
		"host": "192.168.1.42",
		"port": 22,
		"user": "robot",
		"hint": "ssh robot@192.168.1.42",
		"name": id.Name,
	})
}
