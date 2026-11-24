#!/bin/bash
# Source ROS + the zara_interfaces overlay, then run the app command.
set -e
source /opt/ros/humble/setup.bash
source /opt/zara/install/setup.bash
# setup.bash resets PYTHONPATH; keep the SDK importable.
export PYTHONPATH="/opt/zara/python${PYTHONPATH:+:$PYTHONPATH}"
exec "$@"
