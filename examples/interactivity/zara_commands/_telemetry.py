"""Telemetry: live values an app publishes for dashboards (app -> Driver Station).

    zc.publish("position", 12.3)

Semantics are "latest value wins" per key: if a viewer reads slower than the
app publishes, intermediate values for a key are dropped and the viewer gets
the newest one. A viewer that connects late first receives a snapshot of every
key's latest value. This keeps dashboards current instead of falling behind.

Transport-agnostic: backends call subscribe() and stream what it returns.
"""

import json
import threading


class Subscriber:
    """One viewer's queue of pending updates (at most one entry per key)."""

    def __init__(self):
        self._pending = {}
        self._cond = threading.Condition()

    def _offer(self, key, value):
        with self._cond:
            self._pending[key] = value
            self._cond.notify()

    def wait(self, timeout):
        """Block until updates arrive or timeout. Returns {key: value} (may be empty)."""
        with self._cond:
            if not self._pending:
                self._cond.wait(timeout)
            batch, self._pending = self._pending, {}
            return batch


class Hub:
    def __init__(self):
        self._lock = threading.Lock()
        self._latest = {}
        self._subs = set()

    def publish(self, key, value):
        if not isinstance(key, str) or not key:
            raise ValueError("telemetry key must be a non-empty string")
        json.dumps(value)  # fail fast in the app, not later in the stream
        with self._lock:
            self._latest[key] = value
            subs = list(self._subs)
        for s in subs:
            s._offer(key, value)

    def subscribe(self):
        """Returns (subscriber, snapshot_of_latest_values)."""
        s = Subscriber()
        with self._lock:
            self._subs.add(s)
            snapshot = dict(self._latest)
        return s, snapshot

    def unsubscribe(self, s):
        with self._lock:
            self._subs.discard(s)

    def latest(self):
        with self._lock:
            return dict(self._latest)


hub = Hub()
