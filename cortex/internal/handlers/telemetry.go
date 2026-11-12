package handlers

// Telemetry: live values an app publishes (zc.publish) streamed to the
// Driver Station. Cortex proxies the app's loopback SSE stream so the
// Driver Station only ever talks to cortex, behind cortex auth.
//
//	GET /instances/{id}/telemetry[?max_hz=N]
//
// Events (SSE):
//
//	event: snapshot   data: {"ts": <unix ms>, "values": {key: value, ...}}  (once, on connect)
//	event: telemetry  data: {"ts": <unix ms>, "values": {key: value, ...}}  (changed keys only)
//
// Values are "latest wins" per key: a slow viewer skips intermediate values
// rather than falling behind. max_hz (0-1000, 0 = uncapped) limits how often
// this viewer receives batches.

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"cortex/internal/store"
)

// telemetrySource is implemented by transports that can stream telemetry.
type telemetrySource interface {
	OpenTelemetry(ctx context.Context, inst *store.Instance, maxHz string) (*http.Response, error)
}

func (t *directHTTPTransport) OpenTelemetry(ctx context.Context, inst *store.Instance, maxHz string) (*http.Response, error) {
	if inst.CommandPort == 0 {
		return nil, fmt.Errorf("instance has no command port")
	}
	u := fmt.Sprintf("http://127.0.0.1:%d/events", inst.CommandPort)
	if maxHz != "" {
		u += "?max_hz=" + url.QueryEscape(maxHz)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	// The shared client has no overall timeout, which a long-lived stream needs;
	// the request context (client disconnect) ends it.
	return t.client.Do(req)
}

// Telemetry handles GET /instances/{id}/telemetry.
func (h *InstancesHandler) Telemetry(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.runningInstance(w, r)
	if !ok {
		return
	}
	src, ok := h.commands.(telemetrySource)
	if !ok {
		writeError(w, http.StatusNotImplemented,
			"telemetry is not supported by the "+h.commands.Name()+" transport")
		return
	}

	maxHz := r.URL.Query().Get("max_hz")
	if maxHz != "" {
		if v, err := strconv.ParseFloat(maxHz, 64); err != nil || v < 0 || v > 1000 {
			writeError(w, http.StatusBadRequest, "max_hz must be a number from 0 to 1000 (0 = uncapped)")
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	upstream, err := src.OpenTelemetry(r.Context(), inst, maxHz)
	if err != nil {
		writeError(w, transportErrorStatus(err), "could not reach instance telemetry: "+err.Error())
		return
	}
	defer upstream.Body.Close()

	if upstream.StatusCode != http.StatusOK {
		msg := "instance telemetry returned " + upstream.Status
		if upstream.StatusCode == http.StatusNotFound {
			msg = "app does not support telemetry (SDK too old?)"
		}
		writeError(w, upstream.StatusCode, msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	h.log.Event("TELEMETRY STREAM OPENED", "id", inst.ID, "max_hz", maxHz)
	defer h.log.Event("TELEMETRY STREAM CLOSED", "id", inst.ID)

	// Relay line by line, flushing at each event boundary (blank line).
	rd := bufio.NewReader(upstream.Body)
	for {
		line, err := rd.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return // viewer disconnected
			}
			if len(line) == 1 || (len(line) == 2 && line[0] == '\r') {
				flusher.Flush()
			}
		}
		if err != nil {
			if r.Context().Err() == nil {
				// App closed the stream (stopped/crashed): tell the viewer.
				fmt.Fprintf(w, "event: error\ndata: {\"message\":\"instance telemetry stream ended\"}\n\n")
				flusher.Flush()
			}
			return
		}
	}
}
