package handlers

import (
	"encoding/json"
	"net"
	"net/http"

	"cortex/internal/auth"
	"cortex/internal/logger"
	"cortex/internal/store"
)

type AuthHandler struct {
	auth  *auth.Store
	store *store.Store
	log   *logger.Logger
}

func NewAuthHandler(a *auth.Store, s *store.Store, l *logger.Logger) *AuthHandler {
	return &AuthHandler{auth: a, store: s, log: l}
}

// RegisterPublic registers the unauthenticated pairing endpoints.
func (h *AuthHandler) RegisterPublic(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/pair/start", h.PairStart)
	mux.HandleFunc("POST /auth/pair/complete", h.PairComplete)
	mux.HandleFunc("GET /internal/auth/pending", h.PairPending)
}

// RegisterProtected registers the auth management endpoints (require signing).
func (h *AuthHandler) RegisterProtected(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/sessions", h.Sessions)
	mux.HandleFunc("POST /auth/revoke", h.Revoke)
}

func (h *AuthHandler) PairStart(w http.ResponseWriter, r *http.Request) {
	code, expiresIn, err := h.auth.StartPair()
	if err != nil {
		h.log.Error("pair start failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate code")
		return
	}
	h.log.Event("PAIR CODE GENERATED",
		"code", code,
		"expires_in", expiresIn,
		"hint", "submit via POST /auth/pair/complete",
	)
	writeJSON(w, http.StatusOK, map[string]any{"expires_in": expiresIn})
}

// PairPending returns the current pending pairing code for the on-device
// ROS2 bridge to display on the robot's screen. Restricted to localhost only —
// the code must never be readable over the network; it should only be visible
// by physically looking at the robot's display.
func (h *AuthHandler) PairPending(w http.ResponseWriter, r *http.Request) {
	// Strict localhost-only guard: reject anything not from loopback.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
		writeError(w, http.StatusForbidden, "endpoint restricted to localhost")
		return
	}

	pending := h.auth.GetPending()
	if pending == nil {
		writeError(w, http.StatusNotFound, "no pending code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":       pending.Code,
		"expires_at": pending.ExpiresAt,
	})
}

func (h *AuthHandler) PairComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing required field: code")
		return
	}

	salt, createdAt, err := h.auth.CompletePair(req.Code, req.Label)
	if err != nil {
		h.log.Warn("pair complete failed", "reason", err.Error())
		writeError(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}

	label := req.Label
	if label == "" {
		label = "Web Client"
	}

	h.log.Event("AUTH PAIRED", "label", label)
	h.store.AddEvent("auth_paired", map[string]any{"label": label})

	writeJSON(w, http.StatusOK, map[string]any{
		"salt":       salt,
		"label":      label,
		"created_at": createdAt,
	})
}

func (h *AuthHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.auth.ListSessions()
	out := make([]map[string]any, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, map[string]any{
			"token_prefix": sess.Prefix,
			"label":        sess.Label,
			"created_at":   sess.CreatedAt,
			"last_seen":    sess.LastSeen,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *AuthHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TokenPrefix string `json:"token_prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TokenPrefix == "" {
		writeError(w, http.StatusBadRequest, "missing required field: token_prefix")
		return
	}
	if !h.auth.Revoke(req.TokenPrefix) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	h.log.Event("AUTH REVOKED", "prefix", req.TokenPrefix)
	h.store.AddEvent("auth_revoked", map[string]any{"token_prefix": req.TokenPrefix})
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}
