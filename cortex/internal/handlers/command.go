package handlers

// Interactivity spike (#75): POST /instances/{id}/command with two
// interchangeable transports so they can be benchmarked against each other.
//
//   http — cortex forwards to the app's own command server on a loopback
//          port that cortex allocates per instance (ZARA_COMMAND_PORT).
//   ros  — cortex forwards to a single command-bridge app, which calls the
//          ROS 2 service /zara/<instance>/command exposed by the app.
//
// The Driver Station request/response and the app's handler code are the same
// for both, so only the transport differs between benchmark runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"cortex/internal/store"
)

const (
	defaultCommandPortMin = 9100
	defaultCommandPortMax = 9199
	defaultBridgeURL      = "http://127.0.0.1:9000"
	commandTimeout        = 5 * time.Second
)

var commandNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// commandTransport delivers a command to an instance and returns the app's
// raw JSON reply and the HTTP-equivalent status it reported.
type commandTransport interface {
	Name() string
	Send(ctx context.Context, inst *store.Instance, command string, args []byte) (status int, reply []byte, err error)
}

// newCommandTransport picks the transport from CORTEX_COMMAND_TRANSPORT
// (http | ros, default http).
func newCommandTransport() commandTransport {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        128,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	switch os.Getenv("CORTEX_COMMAND_TRANSPORT") {
	case "ros":
		bridge := os.Getenv("CORTEX_COMMAND_BRIDGE_URL")
		if bridge == "" {
			bridge = defaultBridgeURL
		}
		return &rosBridgeTransport{client: client, bridgeURL: bridge}
	default:
		return &directHTTPTransport{client: client}
	}
}

type directHTTPTransport struct{ client *http.Client }

func (t *directHTTPTransport) Name() string { return "http" }

func (t *directHTTPTransport) Send(ctx context.Context, inst *store.Instance, command string, args []byte) (int, []byte, error) {
	if inst.CommandPort == 0 {
		return 0, nil, errors.New("instance has no command port")
	}
	u := fmt.Sprintf("http://127.0.0.1:%d/commands/%s", inst.CommandPort, url.PathEscape(command))
	return postJSON(ctx, t.client, u, args)
}

type rosBridgeTransport struct {
	client    *http.Client
	bridgeURL string
}

func (t *rosBridgeTransport) Name() string { return "ros" }

func (t *rosBridgeTransport) Send(ctx context.Context, inst *store.Instance, command string, args []byte) (int, []byte, error) {
	u := fmt.Sprintf("%s/commands/%s/%s", t.bridgeURL, url.PathEscape(inst.ID), url.PathEscape(command))
	return postJSON(ctx, t.client, u, args)
}

func postJSON(ctx context.Context, c *http.Client, u string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, reply, err
}

// commandPortRange reads CORTEX_COMMAND_PORT_MIN/MAX.
func commandPortRange() (int, int) {
	lo, hi := defaultCommandPortMin, defaultCommandPortMax
	if v, err := strconv.Atoi(os.Getenv("CORTEX_COMMAND_PORT_MIN")); err == nil && v > 0 {
		lo = v
	}
	if v, err := strconv.Atoi(os.Getenv("CORTEX_COMMAND_PORT_MAX")); err == nil && v >= lo {
		hi = v
	}
	return lo, hi
}

// loopbackPortFree reports whether nothing is bound to 127.0.0.1:p. Instances
// run with --network host, so this is the same namespace the app will use.
func loopbackPortFree(p int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// commandEnv allocates a command port for the instance and returns the env
// vars the app SDK reads. Allocation failure is logged, not fatal: the app
// still starts, it just can't receive commands over the http transport.
func (h *InstancesHandler) commandEnv(instanceID string) map[string]string {
	env := map[string]string{
		"ZARA_INSTANCE_ID":       instanceID,
		"ZARA_COMMAND_TRANSPORT": h.commands.Name(),
	}
	lo, hi := commandPortRange()
	port, err := h.store.AllocateCommandPort(instanceID, lo, hi, loopbackPortFree)
	if err != nil {
		h.log.Warn("command port allocation failed", "id", instanceID, "err", err)
		return env
	}
	env["ZARA_COMMAND_PORT"] = strconv.Itoa(port)
	return env
}

// Command handles POST /instances/{id}/command.
//
// Request:  {"command": "move", "args": {...}}
// Response: {"instance", "command", "transport", "forward_us", "reply": <app reply>}
// where the app reply is {"ok", "result", "error", "handler_us"}.
func (h *InstancesHandler) Command(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := h.store.GetInstance(id)
	if !ok {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if inst.State != "running" {
		writeError(w, http.StatusConflict, "instance is "+inst.State)
		return
	}

	var req struct {
		Command string          `json:"command"`
		Args    json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if !commandNameRe.MatchString(req.Command) {
		writeError(w, http.StatusBadRequest, "command must match "+commandNameRe.String())
		return
	}
	args := []byte(req.Args)
	if len(args) == 0 || string(args) == "null" {
		args = []byte("{}")
	}

	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()

	start := time.Now()
	status, reply, err := h.commands.Send(ctx, inst, req.Command, args)
	forwardUS := time.Since(start).Microseconds()

	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			code = http.StatusGatewayTimeout
		}
		writeJSON(w, code, map[string]any{
			"instance":   id,
			"command":    req.Command,
			"transport":  h.commands.Name(),
			"forward_us": forwardUS,
			"error":      "could not reach instance command server: " + err.Error(),
		})
		return
	}

	out := map[string]any{
		"instance":   id,
		"command":    req.Command,
		"transport":  h.commands.Name(),
		"forward_us": forwardUS,
	}
	if json.Valid(reply) {
		out["reply"] = json.RawMessage(reply)
	} else {
		out["reply"] = string(reply)
	}
	writeJSON(w, status, out)
}
