#!/bin/sh
case "$1" in
    start) node_exporter --web.listen-address=":9100" &;;
    stop)  killall node_exporter 2>/dev/null || true;;
    *)     echo "Usage: $0 {start|stop}"; exit 1;;
esac
