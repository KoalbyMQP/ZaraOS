#!/bin/sh

case "$1" in
    start)
        echo "Starting node_exporter..."
        node_exporter --web.listen-address=":9100" < /dev/null &
        ;;
    stop)
        killall node_exporter 2>/dev/null || true
        ;;
    *)
        echo "Usage: $0 {start|stop}"
        exit 1
        ;;
esac
