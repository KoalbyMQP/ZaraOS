"""Option B: one HTTP -> ROS 2 bridge for all apps.

Cortex (CORTEX_COMMAND_TRANSPORT=ros) forwards
    POST /instances/<id>/command {"command": c, "args": a}
to
    POST http://127.0.0.1:<ZARA_BRIDGE_PORT>/commands/<id>/<c>   body: a
and this bridge calls the ROS service /zara/<id>/command on the target app.
The reply body is passed through unchanged, so cortex's response has the
same shape as with the direct HTTP transport.
"""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import unquote

import rclpy
from rclpy.executors import MultiThreadedExecutor
from rclpy.node import Node
from zara_interfaces.srv import Command

from zara_commands import service_name

PORT = int(os.environ.get("ZARA_BRIDGE_PORT", "9000"))
DISCOVERY_TIMEOUT_S = float(os.environ.get("ZARA_BRIDGE_DISCOVERY_TIMEOUT", "2.0"))
CALL_TIMEOUT_S = float(os.environ.get("ZARA_BRIDGE_CALL_TIMEOUT", "4.0"))

rclpy.init()
node = Node("zara_command_bridge")
executor = MultiThreadedExecutor()
executor.add_node(node)
threading.Thread(target=executor.spin, name="ros-spin", daemon=True).start()

_clients = {}
_clients_lock = threading.Lock()


def get_client(instance_id):
    with _clients_lock:
        client = _clients.get(instance_id)
        if client is None:
            client = node.create_client(Command, service_name(instance_id))
            _clients[instance_id] = client
        return client


def call(instance_id, command, args_json):
    """Returns (status, body_bytes)."""
    client = get_client(instance_id)
    if not client.service_is_ready() and not client.wait_for_service(timeout_sec=DISCOVERY_TIMEOUT_S):
        return 503, _err("no command service for instance %s (%s)"
                         % (instance_id, service_name(instance_id)))

    req = Command.Request()
    req.command = command
    req.args_json = args_json
    done = threading.Event()
    future = client.call_async(req)
    future.add_done_callback(lambda _f: done.set())
    if not done.wait(CALL_TIMEOUT_S):
        client.remove_pending_request(future)
        return 504, _err("timed out waiting for instance %s" % instance_id)
    resp = future.result()
    return resp.status, resp.reply_json.encode()


def _err(msg):
    return json.dumps({"ok": False, "result": None, "error": msg, "handler_us": 0}).encode()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _send(self, status, data):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length).decode() if length else "{}"
        parts = self.path.strip("/").split("/")
        if len(parts) != 3 or parts[0] != "commands":
            self._send(404, _err("expected /commands/<instance>/<command>"))
            return
        instance_id, command = unquote(parts[1]), unquote(parts[2])
        status, data = call(instance_id, command, body)
        self._send(status, data)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    # Loopback only: cortex is the only intended caller.
    server = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    server.daemon_threads = True
    print("[command-bridge] listening on 127.0.0.1:%d" % PORT, flush=True)
    try:
        server.serve_forever()
    finally:
        executor.shutdown()
        rclpy.shutdown()
