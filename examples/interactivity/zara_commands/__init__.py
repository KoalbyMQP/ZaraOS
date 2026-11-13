"""Transport-agnostic command handling for ZaraOS apps (interactivity spike, #75).

App code is identical for both transports:

    import zara_commands as zc

    @zc.command()
    def move(speed: float = 1.0):
        ...
        return {"moving": True}

    zc.start()   # non-blocking; picks the transport from ZARA_COMMAND_TRANSPORT

    zc.publish("position", 12.3)   # live value for dashboards (latest wins)

    @zc.control(min=0, max=100, default=50)   # slider: called with the newest value only
    def speed(value: float):
        ...

Every handler reply has the same shape regardless of transport:
    {"ok": bool, "result": <any JSON>, "error": str | None, "handler_us": int}
"""

import inspect
import json
import os
import re
import threading
import time

from . import _controls
from ._controls import control
from ._telemetry import hub as _hub

__all__ = ["command", "commands", "control", "dispatch", "publish", "start", "service_name"]


def publish(key, value):
    """Publish a live value (any JSON-serializable value) for dashboards.

    Cheap to call often: viewers only ever receive the latest value per key.
    """
    _hub.publish(key, value)

_handlers = {}
_lock = threading.Lock()

# Built-in commands answered by the SDK itself. Names starting with "_" are
# reserved for these; cortex rejects them in POST /command, so the Driver
# Station reaches them only through dedicated endpoints.
DESCRIBE = "_commands"            # GET /instances/{id}/commands
DESCRIBE_CONTROLS = "_controls"   # GET /instances/{id}/controls
SET_CONTROL = "_set_control"      # PUT /instances/{id}/controls/{name}


def command(name=None):
    """Register a function as a command. The command name defaults to the function name."""

    def deco(fn):
        cmd = name or fn.__name__
        if cmd.startswith("_"):
            raise ValueError("command names starting with '_' are reserved: %r" % cmd)
        with _lock:
            _handlers[cmd] = fn
        return fn

    return deco


def commands():
    """Describe registered commands (name + parameters), e.g. for dashboard generation."""
    with _lock:
        items = list(_handlers.items())
    out = []
    for cmd, fn in items:
        params = []
        for p in inspect.signature(fn).parameters.values():
            param = {"name": p.name, "required": p.default is inspect.Parameter.empty}
            if p.annotation is not inspect.Parameter.empty:
                param["type"] = getattr(p.annotation, "__name__", str(p.annotation))
            if p.default is not inspect.Parameter.empty:
                param["default"] = _json_safe(p.default)
            params.append(param)
        out.append({"name": cmd, "params": params, "doc": inspect.getdoc(fn)})
    return out


def _json_safe(value):
    """Defaults go over the wire as JSON; fall back to repr for anything else."""
    try:
        json.dumps(value)
        return value
    except (TypeError, ValueError):
        return repr(value)


def _reply(status, ok, result=None, error=None, t0=None):
    handler_us = (time.perf_counter_ns() - t0) // 1000 if t0 is not None else 0
    return status, {"ok": ok, "result": result, "error": error, "handler_us": handler_us}


def dispatch(name, args):
    """Run a command. Returns (http_status, reply_dict). Never raises."""
    if name == DESCRIBE:
        return _reply(200, True, result=commands())
    if name == DESCRIBE_CONTROLS:
        return _reply(200, True, result=[c.describe() for c in _controls.all_controls()])
    if name == SET_CONTROL:
        return _set_control(args)
    with _lock:
        fn = _handlers.get(name)
    if fn is None:
        return _reply(404, False, error="unknown command: %s" % name)
    if args is None:
        args = {}
    if not isinstance(args, dict):
        return _reply(400, False, error="args must be a JSON object")
    try:
        bound = inspect.signature(fn).bind(**args)
    except TypeError as e:
        return _reply(400, False, error="bad args: %s" % e)

    t0 = time.perf_counter_ns()
    try:
        result = fn(*bound.args, **bound.kwargs)
    except Exception as e:  # noqa: BLE001 - report any handler failure to the caller
        return _reply(500, False, error="%s: %s" % (type(e).__name__, e), t0=t0)
    return _reply(200, True, result=result, t0=t0)


def _set_control(args):
    """args: {"name": str, "value": any, "seq": int | null}. Returns immediately."""
    if not isinstance(args, dict) or "name" not in args or "value" not in args:
        return _reply(400, False, error="expected {name, value, seq?}")
    c = _controls.get(args["name"])
    if c is None:
        return _reply(404, False, error="unknown control: %s" % args["name"])
    seq = args.get("seq")
    if seq is not None and (isinstance(seq, bool) or not isinstance(seq, int)):
        return _reply(400, False, error="seq must be an integer")
    try:
        accepted, value = c.set(args["value"], seq)
    except _controls.ControlError as e:
        return _reply(400, False, error=str(e))
    return _reply(200, True, result={"accepted": accepted, "stale": not accepted, "value": value})


def ros_name(instance_id):
    """ROS-safe name token for an instance id (ids contain '-', which ROS rejects)."""
    return "i_" + re.sub(r"[^A-Za-z0-9_]", "_", instance_id)


def service_name(instance_id):
    return "/zara/%s/command" % ros_name(instance_id)


def start(transport=None):
    """Start receiving commands in a background thread. Returns a stop() callable."""
    transport = transport or os.environ.get("ZARA_COMMAND_TRANSPORT", "http")
    if transport == "http":
        from . import _http as backend
    elif transport == "ros":
        from . import _ros as backend
    else:
        raise ValueError("unknown ZARA_COMMAND_TRANSPORT: %r" % transport)
    stop = backend.start()
    _controls.start_all()
    print("[zara_commands] %s transport ready, commands: %s; controls: %s"
          % (transport, ", ".join(sorted(_handlers)) or "-",
             ", ".join(sorted(c.name for c in _controls.all_controls())) or "-"), flush=True)
    return stop
