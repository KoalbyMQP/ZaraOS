"""Controls: continuously-adjustable values driven from the Driver Station (sliders, toggles).

    @zc.control(min=0, max=100, step=1, default=50)
    def speed(value: float):
        motor.set_speed(value)

Unlike commands, control updates are "latest value wins":
  - set() returns immediately; the handler runs on the control's own worker thread.
  - If several updates arrive while the handler is busy, it runs once more with
    only the newest value, so the app never falls behind a moving slider.
  - Each update may carry a sequence number; an update older than the last one
    accepted is dropped, so out-of-order delivery can't move the value backwards.
  - After the handler applies a value it is published as telemetry
    "controls.<name>", so every viewer sees what the app actually applied.
"""

import inspect
import sys
import threading

from ._telemetry import hub

_TYPES = {"float": float, "int": int, "bool": bool, "str": str}


class ControlError(ValueError):
    pass


class Control:
    def __init__(self, name, fn, vtype, minimum, maximum, step, default, doc):
        self.name, self.fn, self.type = name, fn, vtype
        self.min, self.max, self.step, self.default = minimum, maximum, step, default
        self.doc = doc
        self._lock = threading.Lock()
        self._wake = threading.Event()
        self._requested = default    # newest accepted value
        self._applied = None         # last value the handler finished with
        self._seq = None             # seq of the newest accepted update
        self._calls = 0
        self._started = False

    def coerce(self, value):
        conv = _TYPES.get(self.type)
        try:
            if conv is bool and not isinstance(value, bool):
                raise TypeError
            if conv is not None:
                value = conv(value)
        except (TypeError, ValueError):
            raise ControlError("value for %s must be %s" % (self.name, self.type))
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            if self.min is not None and value < self.min:
                raise ControlError("%s must be >= %s" % (self.name, self.min))
            if self.max is not None and value > self.max:
                raise ControlError("%s must be <= %s" % (self.name, self.max))
        return value

    def set(self, value, seq=None):
        """Accept a new value. Returns (accepted, coerced_value). Raises ControlError."""
        value = self.coerce(value)
        with self._lock:
            if seq is not None and self._seq is not None and seq <= self._seq:
                return False, self._requested  # stale: older than what we already have
            if seq is not None:
                self._seq = seq
            self._requested = value
        self._wake.set()
        return True, value

    def describe(self):
        with self._lock:
            return {
                "name": self.name, "type": self.type, "min": self.min, "max": self.max,
                "step": self.step, "default": self.default,
                "value": self._applied, "doc": self.doc,
                "handler_calls": self._calls, "last_seq": self._seq,
            }

    def start(self):
        if self._started:
            return
        self._started = True
        if self._requested is not None:
            self._wake.set()  # apply the default once at startup
        threading.Thread(target=self._run, name="zara-control-" + self.name, daemon=True).start()

    def _run(self):
        while True:
            self._wake.wait()
            self._wake.clear()
            with self._lock:
                value = self._requested
            if value is None:
                continue
            try:
                self.fn(value)
            except Exception as e:  # noqa: BLE001 - a bad handler must not kill the worker
                print("[zara_commands] control %s handler failed: %s: %s"
                      % (self.name, type(e).__name__, e), file=sys.stderr, flush=True)
                continue
            with self._lock:
                self._applied = value
                self._calls += 1
            hub.publish("controls." + self.name, value)


_controls = {}
_lock = threading.Lock()


def control(name=None, min=None, max=None, step=None, default=None):  # noqa: A002 - match slider vocabulary
    """Register a function as a control. It is called with the newest value.

    The value type comes from the parameter annotation (float, int, bool, str).
    """

    def deco(fn):
        cname = name or fn.__name__
        if cname.startswith("_"):
            raise ValueError("control names starting with '_' are reserved: %r" % cname)
        params = list(inspect.signature(fn).parameters.values())
        if len(params) != 1:
            raise ValueError("control %r must take exactly one parameter (the value)" % cname)
        ann = params[0].annotation
        vtype = getattr(ann, "__name__", None) if ann is not inspect.Parameter.empty else None
        if vtype is not None and vtype not in _TYPES:
            raise ValueError("control %r: unsupported type %r (use float, int, bool or str)" % (cname, vtype))
        c = Control(cname, fn, vtype, min, max, step, None, inspect.getdoc(fn))
        if default is not None:
            c.default = c._requested = c.coerce(default)
        with _lock:
            _controls[cname] = c
        return fn

    return deco


def get(name):
    with _lock:
        return _controls.get(name)


def all_controls():
    with _lock:
        return list(_controls.values())


def start_all():
    for c in all_controls():
        c.start()
