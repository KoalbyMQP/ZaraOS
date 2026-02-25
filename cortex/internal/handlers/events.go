package handlers

import (
	"net/http"
	"strconv"
	"time"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type EventsHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewEventsHandler(s *store.Store, l *logger.Logger) *EventsHandler {
	return &EventsHandler{store: s, log: l}
}

func (h *EventsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /events", h.List)
}

func (h *EventsHandler) List(w http.ResponseWriter, r *http.Request) {
	var since *time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err == nil {
			since = &t
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	events := h.store.ListEvents(since, r.URL.Query().Get("type"), limit)
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		out = append(out, map[string]any{
			"id":        ev.ID,
			"type":      ev.Type,
			"timestamp": ev.Timestamp,
			"data":      ev.Data,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}
