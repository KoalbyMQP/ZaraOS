#!/bin/sh
case "$1" in
    start)
        mkdir -p /var/run/dbus
        dbus-uuidgen --ensure
        dbus-daemon --system --fork
        ;;
    stop) killall dbus-daemon 2>/dev/null || true ;;
    *) echo "Usage: $0 {start|stop}"; exit 1 ;;
esac
