package handlers

import (
	"net/http"
	"strconv"
	"time"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type DiagnosticsHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewDiagnosticsHandler(s *store.Store, l *logger.Logger) *DiagnosticsHandler {
	return &DiagnosticsHandler{store: s, log: l}
}

func (h *DiagnosticsHandler) Register(mux *http.ServeMux) {
	// /diagnostics/history must be registered before /diagnostics/{test}
	// so the fixed path wins on exact match.
	mux.HandleFunc("GET /diagnostics", h.RunAll)
	mux.HandleFunc("GET /diagnostics/history", h.History)
	mux.HandleFunc("GET /diagnostics/{test}", h.GetResult)
	mux.HandleFunc("POST /diagnostics/{test}/run", h.RunOne)
}

func (h *DiagnosticsHandler) RunAll(w http.ResponseWriter, r *http.Request) {
	tests := h.store.ListDiagTests()
	results := make([]map[string]any, 0, len(tests))
	passed, failed := 0, 0

	for _, t := range tests {
		result := h.execute(t)
		if result.Passed {
			passed++
		} else {
			failed++
		}
		results = append(results, diagResultMap(&result))
	}

	h.log.Event("DIAGNOSTICS RUN ALL", "passed", passed, "failed", failed, "total", len(tests))
	h.store.AddEvent("diagnostic_run", map[string]any{"passed": passed, "failed": failed})

	writeJSON(w, http.StatusOK, map[string]any{
		"ran_at":  time.Now().UTC(),
		"passed":  passed,
		"failed":  failed,
		"results": results,
	})
}

func (h *DiagnosticsHandler) History(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	history := make([]map[string]any, 0, limit)
	now := time.Now().UTC()
	for i := 0; i < limit && i < 5; i++ {
		f, p := i%2, 4-i%2
		history = append(history, map[string]any{
			"ran_at": now.Add(-time.Duration(i) * time.Hour),
			"passed": p,
			"failed": f,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (h *DiagnosticsHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("test")
	result, ok := h.store.GetDiagResult(name)
	if !ok {
		writeError(w, http.StatusNotFound, "diagnostic test not found")
		return
	}
	writeJSON(w, http.StatusOK, diagResultMap(result))
}

func (h *DiagnosticsHandler) RunOne(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("test")
	t, ok := h.store.GetDiagTest(name)
	if !ok {
		writeError(w, http.StatusNotFound, "diagnostic test not found")
		return
	}
	result := h.execute(t)
	h.log.Event("DIAGNOSTIC RAN",
		"test", name,
		"passed", result.Passed,
		"duration_ms", result.DurationMs,
		"output", result.Output,
	)
	writeJSON(w, http.StatusOK, diagResultMap(&result))
}

func (h *DiagnosticsHandler) execute(t store.DiagTest) store.DiagResult {
	output := t.FailedOutput
	if t.DefaultPassing {
		output = t.PassedOutput
	}
	result := store.DiagResult{
		Name:       t.Name,
		Passed:     t.DefaultPassing,
		Output:     output,
		DurationMs: t.DurationMs,
		RanAt:      time.Now().UTC(),
	}
	h.store.SetDiagResult(result)
	return result
}

func diagResultMap(r *store.DiagResult) map[string]any {
	return map[string]any{
		"name":        r.Name,
		"passed":      r.Passed,
		"output":      r.Output,
		"duration_ms": r.DurationMs,
		"ran_at":      r.RanAt,
	}
}
