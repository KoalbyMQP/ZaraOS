#!/bin/sh

case "$1" in
    start)
        # Wait for first-boot setup (with timeout so cortex starts regardless).
        # On first boot the setup wizard configures WiFi — cortex can
        # still serve local health checks and management without it.
        if [ ! -f /data/.setup-done ]; then
            echo "Waiting for setup wizard (max 60s)..."
            WAIT=60
            while [ ! -f /data/.setup-done ] && [ $WAIT -gt 0 ]; do
                sleep 2
                WAIT=$((WAIT - 2))
            done
            [ -f /data/.setup-done ] && echo "Setup done." || echo "Setup not done yet, starting cortex anyway."
        fi

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
