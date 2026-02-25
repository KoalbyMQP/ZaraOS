package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"cortex/internal/logger"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type ticket struct {
	createdAt time.Time
}

type ShellHandler struct {
	log     *logger.Logger
	mu      sync.Mutex
	tickets map[string]ticket
	upgrader websocket.Upgrader
}

func NewShellHandler(l *logger.Logger) *ShellHandler {
	h := &ShellHandler{
		log:     l,
		tickets: make(map[string]ticket),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	go h.cleanTickets()
	return h
}

func (h *ShellHandler) Register(prot, pub *http.ServeMux) {
	prot.HandleFunc("POST /shell/ticket", h.IssueTicket)
	pub.HandleFunc("GET /shell", h.Shell)
}

// IssueTicket is called on the authenticated mux.
// Returns a one-time token valid for 30 seconds.
func (h *ShellHandler) IssueTicket(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate ticket")
		return
	}
	tok := base64.RawURLEncoding.EncodeToString(b)

	h.mu.Lock()
	h.tickets[tok] = ticket{createdAt: time.Now()}
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     tok,
		"expires_in": 30,
	})
}

// Shell handles the WebSocket upgrade. Auth via ticket query param.
func (h *ShellHandler) Shell(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("ticket")
	if tok == "" {
		http.Error(w, "missing ticket", http.StatusUnauthorized)
		return
	}

	h.mu.Lock()
	t, ok := h.tickets[tok]
	if ok {
		delete(h.tickets, tok) // one-use
	}
	h.mu.Unlock()

	if !ok || time.Since(t.createdAt) > 30*time.Second {
		http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	shell := exec.Command("/bin/sh")
	ptmx, err := pty.Start(shell)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("failed to start shell: "+err.Error()))
		return
	}
	defer ptmx.Close()
	defer shell.Process.Kill()

	// PTY → WS
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WS → PTY
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.TextMessage {
			// control message: {"type":"resize","cols":N,"rows":N}
			var ctrl struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(msg, &ctrl) == nil && ctrl.Type == "resize" {
				pty.Setsize(ptmx, &pty.Winsize{Cols: ctrl.Cols, Rows: ctrl.Rows})
			}
		} else {
			ptmx.Write(msg)
		}
	}
}

func (h *ShellHandler) cleanTickets() {
	for range time.Tick(60 * time.Second) {
		h.mu.Lock()
		for tok, t := range h.tickets {
			if time.Since(t.createdAt) > 60*time.Second {
				delete(h.tickets, tok)
			}
		}
		h.mu.Unlock()
	}
}
