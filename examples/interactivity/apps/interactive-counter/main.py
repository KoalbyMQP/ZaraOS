"""Example interactive app: a counter that ticks in a loop and accepts commands.

The same code runs under both transports; only ZARA_COMMAND_TRANSPORT
(set by cortex) changes. `ping` is the no-op command used for latency benchmarks.
"""

import threading
import time

import zara_commands as zc

_lock = threading.Lock()
state = {"count": 0, "rate_hz": 1.0, "step": 1}


@zc.command()
def ping():
    """No-op round trip, used by the benchmark."""
    return "pong"


@zc.command()
def this_should_be_a_new_command(arg1: int, arg2: float):
    """this should be discovered by the command discovery"""
    return arg1 + arg2


@zc.command()
def add(n: int = 1):
    """Add n to the counter immediately."""
    with _lock:
        state["count"] += int(n)
        return state["count"]


@zc.command()
def set_rate(hz: float):
    """How many times per second the counter ticks (0.1 - 100)."""
    hz = float(hz)
    if not 0.1 <= hz <= 100:
        raise ValueError("hz must be between 0.1 and 100")
    with _lock:
        state["rate_hz"] = hz
    return hz


@zc.command()
def set_step(step: int):
    """How much each tick adds."""
    with _lock:
        state["step"] = int(step)
    return state["step"]


@zc.command()
def get_state():
    with _lock:
        return dict(state)


def main():
    zc.start()
    last_print = 0.0
    while True:
        with _lock:
            state["count"] += state["step"]
            count, rate = state["count"], state["rate_hz"]
        now = time.monotonic()
        if now - last_print >= 1.0:  # log at most once a second regardless of rate
            print("count=%d rate_hz=%g" % (count, rate), flush=True)
            last_print = now
        time.sleep(1.0 / rate)


if __name__ == "__main__":
    main()
