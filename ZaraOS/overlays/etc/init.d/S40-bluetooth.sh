#!/bin/sh
case "$1" in
    start)
        bluetoothd &
        ;;
    stop) killall bluetoothd 2>/dev/null || true ;;
    *) echo "Usage: $0 {start|stop}"; exit 1 ;;
esac
