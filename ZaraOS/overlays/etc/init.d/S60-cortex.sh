#!/bin/sh

case "$1" in
    start)
        echo "Starting cortex..."

        # Copy default requirements manifest on first boot.
        if [ ! -f /data/config/requirements.json ] && [ -f /etc/zaraos/requirements.json ]; then
            mkdir -p /data/config
            cp /etc/zaraos/requirements.json /data/config/requirements.json
            echo "Copied default requirements manifest to /data/config/"
        fi

        GITHUB_ORG=KoalbyMQP \
        GITHUB_REPOS=Core \
        DOCKERHUB_ORG=koalby \
        REQUIREMENTS_PATH=/data/config/requirements.json \
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
