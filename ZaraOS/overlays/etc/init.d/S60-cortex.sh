#!/bin/sh

case "$1" in
    start)
        echo "Starting cortex..."
        GITHUB_ORG=KoalbyMQP \
        GITHUB_REPOS=Core \
        DOCKERHUB_ORG=koalby \
        cortex < /dev/null &
        ;;
    stop)
        killall cortex 2>/dev/null || true
        ;;
    *)
        echo "Usage: $0 {start|stop}"
        exit 1
        ;;
esac
