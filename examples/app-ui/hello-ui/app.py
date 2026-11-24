"""A small app with a real backend and a bundled UI (see README.md).

Commands, a control and telemetry through zara_commands; the UI under ui/ is built
with Rsbuild + Module Federation and shipped in the image at /etc/zaraos/ui/.
"""
import math
import threading
import time

import zara_commands as zc

_lock = threading.Lock()
_notes: list[str] = []
_rate_hz = 1.0


@zc.command()
def add_note(text: str):
    """Store a note. Returns every note."""
    text = text.strip()
    if not text:
        raise ValueError("a note cannot be empty")
    if len(text) > 200:
        raise ValueError("a note can be at most 200 characters")
    with _lock:
        _notes.append(text)
        return {"notes": list(_notes)}


@zc.command()
def list_notes():
    """Return every note."""
    with _lock:
        return {"notes": list(_notes)}


@zc.command()
def clear_notes():
    """Delete every note."""
    with _lock:
        _notes.clear()
    return {"notes": []}


@zc.control(min=0.2, max=5, step=0.2, default=1)
def wave_hz(value: float):
    """Frequency of the simulated sensor wave."""
    global _rate_hz
    _rate_hz = float(value)
    print(f"wave_hz applied: {value}", flush=True)


zc.start()
started = time.time()
while True:
    now = time.time() - started
    with _lock:
        count = len(_notes)
    zc.publish("uptime_s", int(now))
    zc.publish("notes", count)
    zc.publish("wave", round(math.sin(2 * math.pi * _rate_hz * now), 3))
    time.sleep(0.1)
