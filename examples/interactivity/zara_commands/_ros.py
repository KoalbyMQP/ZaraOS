"""Option B: the app exposes a ROS 2 service /zara/<instance>/command.

The command-bridge app turns cortex's HTTP request into a call to this service.
A per-instance service (not a shared topic) means only the targeted app sees
the command, and gives us a reply channel for free.
"""

import json
import os
import threading

import rclpy
from rclpy.executors import SingleThreadedExecutor
from rclpy.node import Node
from zara_interfaces.srv import Command

from . import dispatch, ros_name, service_name


def start():
    instance_id = os.environ.get("ZARA_INSTANCE_ID")
    if not instance_id:
        raise RuntimeError("ZARA_INSTANCE_ID not set (is this app running under cortex?)")

    if not rclpy.ok():
        rclpy.init()
    node = Node(ros_name(instance_id) + "_commands")

    def on_command(req, resp):
        try:
            args = json.loads(req.args_json) if req.args_json else {}
        except ValueError:
            status, reply = 400, {"ok": False, "result": None,
                                  "error": "args_json is not valid JSON", "handler_us": 0}
        else:
            status, reply = dispatch(req.command, args)
        resp.status = status
        resp.reply_json = json.dumps(reply)
        return resp

    node.create_service(Command, service_name(instance_id), on_command)

    executor = SingleThreadedExecutor()
    executor.add_node(node)
    threading.Thread(target=executor.spin, name="zara-commands-ros", daemon=True).start()

    def stop():
        executor.shutdown()
        node.destroy_node()

    return stop
