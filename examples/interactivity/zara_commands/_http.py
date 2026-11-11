"""Option A: each app runs its own loopback HTTP command server.

Cortex allocates the port per instance and passes it in ZARA_COMMAND_PORT.
Binds to 127.0.0.1 only: instances use host networking, so binding 0.0.0.0
would expose every app's commands to the whole network, bypassing cortex auth.
"""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from . import commands, dispatch

_PREFIX = "/commands/"


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
        if self.path.rstrip("/") == "/commands":
            self._send(200, {"commands": commands()})
        else:
            self._send(404, {"ok": False, "result": None, "error": "not found"})

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
