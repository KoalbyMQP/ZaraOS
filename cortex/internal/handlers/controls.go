package handlers

// Controls: continuously-adjustable app values (sliders, toggles) set from the
// Driver Station. The mirror image of telemetry.
//
//	GET /instances/{id}/controls
//	    -> {"instance", "transport", "controls": [{"name", "type", "min", "max", "step",
//	        "default", "value", "doc", "handler_calls", "last_seq"}]}
//
//	PUT /instances/{id}/controls/{name}   {"value": 42, "seq": 17}
//	    -> {"instance", "control", "accepted", "stale", "value", "forward_us"}
//
// A PUT returns as soon as the app has accepted the value, without waiting for
// the app's handler. The app applies only the newest value ("latest wins"),
// and drops any update whose seq is not newer than the last accepted one, so
// a fast-moving slider never queues up behind a slow handler and out-of-order
// delivery can't move the value backwards. The applied value is published as
// telemetry "controls.<name>".
//
// seq is optional but recommended: a per-slider counter the Driver Station
// increments on every update.

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

const (
	describeControls = "_controls"
	setControl       = "_set_control"
)

// ListControls handles GET /instances/{id}/controls.
func (h *InstancesHandler) ListControls(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.runningInstance(w, r)
	if !ok {
		return
	}
	result, _, ok := h.callBuiltin(w, r, inst, describeControls, []byte("{}"), "controls")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instance":  inst.ID,
		"transport": h.commands.Name(),
		"controls":  orEmptyList(result),
	})
}

// SetControl handles PUT /instances/{id}/controls/{name}.
func (h *InstancesHandler) SetControl(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.runningInstance(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !commandNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "control name must match "+commandNameRe.String())
		return
	}

	var req struct {
		Value json.RawMessage `json:"value"`
		Seq   *int64          `json:"seq"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if len(req.Value) == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: value")
		return
	}

	args, _ := json.Marshal(map[string]any{"name": name, "value": req.Value, "seq": req.Seq})
	result, forwardUS, ok := h.callBuiltin(w, r, inst, setControl, args, "controls")
	if !ok {
		return
	}

	var res struct {
		Accepted bool            `json:"accepted"`
		Stale    bool            `json:"stale"`
		Value    json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		writeError(w, http.StatusBadGateway, "instance returned an invalid reply")
		return
	}
	seq := "-"
	if req.Seq != nil {
		seq = strconv.FormatInt(*req.Seq, 10)
	}
	h.log.Info("CONTROL", "id", inst.ID, "name", name, "value", string(req.Value), "seq", seq,
		"accepted", res.Accepted, "forward_us", forwardUS)
	writeJSON(w, http.StatusOK, map[string]any{
		"instance":   inst.ID,
		"control":    name,
		"accepted":   res.Accepted,
		"stale":      res.Stale,
		"value":      res.Value,
		"forward_us": forwardUS,
	})
}
