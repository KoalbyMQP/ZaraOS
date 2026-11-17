"""Option A: each app runs its own loopback HTTP command server.

Cortex allocates the port per instance and passes it in ZARA_COMMAND_PORT.
Binds to 127.0.0.1 only: instances use host networking, so binding 0.0.0.0
would expose every app's commands to the whole network, bypassing cortex auth.

Endpoints (cortex is the only caller):
    POST /commands/<name>   run a command
    GET  /commands          list commands
    GET  /events[?max_hz=N] SSE telemetry stream: "snapshot" event, then
                            "telemetry" events, each {"ts": ms, "values": {key: value}}
"""

import json
import os
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs

from . import commands, dispatch
from ._telemetry import hub

_PREFIX = "/commands/"
_HEARTBEAT_S = 15.0  # keepalive comment so proxies/clients don't time out idle streams


class _Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"  # keep-alive, so cortex can reuse connections

    def _send(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        path, _, query = self.path.partition("?")
        if path.rstrip("/") == "/commands":
            self._send(200, {"commands": commands()})
        elif path.rstrip("/") == "/events":
            self._stream_events(parse_qs(query))
        else:
            self._send(404, {"ok": False, "result": None, "error": "not found"})

    def _stream_events(self, query):
        """SSE stream of telemetry. ?max_hz=N caps how often batches are sent."""
        try:
            max_hz = float(query.get("max_hz", ["0"])[0])
        except ValueError:
            max_hz = -1
        if max_hz < 0 or max_hz > 1000:
            self._send(400, {"ok": False, "result": None, "error": "max_hz must be 0-1000 (0 = no cap)"})
            return
        min_interval = 1.0 / max_hz if max_hz else 0.0

        self.close_connection = True  # stream runs until the client disconnects
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()

        sub, snapshot = hub.subscribe()
        try:
            self._event("snapshot", snapshot)
            last_sent = time.monotonic()
            while True:
                batch = sub.wait(_HEARTBEAT_S)
                if not batch:
                    self.wfile.write(b": keepalive\n\n")
                    self.wfile.flush()
                    continue
                if min_interval:
                    wait = min_interval - (time.monotonic() - last_sent)
                    if wait > 0:
                        time.sleep(wait)
                        batch.update(sub.wait(0))  # fold in anything newer (latest wins)
                self._event("telemetry", batch)
                last_sent = time.monotonic()
        except (BrokenPipeError, ConnectionResetError, OSError):
            pass  # viewer went away
        finally:
            hub.unsubscribe(sub)

    def _event(self, name, values):
        payload = json.dumps({"ts": int(time.time() * 1000), "values": values})
        self.wfile.write(("event: %s\ndata: %s\n\n" % (name, payload)).encode())
        self.wfile.flush()

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        if not self.path.startswith(_PREFIX):
            self._send(404, {"ok": False, "result": None, "error": "not found"})
            return
        name = self.path[len(_PREFIX):]
        try:
            args = json.loads(raw) if raw else {}
        except ValueError:
            self._send(400, {"ok": False, "result": None, "error": "body is not valid JSON"})
            return
        status, reply = dispatch(name, args)
        self._send(status, reply)

    def log_message(self, *args):  # keep app logs clean; cortex already logs requests
        pass


def start():
    port = int(os.environ.get("ZARA_COMMAND_PORT", "0"))
    if not port:
        raise RuntimeError("ZARA_COMMAND_PORT not set (is this app running under cortex?)")
    server = ThreadingHTTPServer(("127.0.0.1", port), _Handler)
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, name="zara-commands-http", daemon=True).start()

    def stop():
        server.shutdown()
        server.server_close()

    return stop
