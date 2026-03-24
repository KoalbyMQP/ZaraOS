#!/usr/bin/env bash
set -euo pipefail

# 1. Start Docker-in-Docker and wait until it is ready.
echo "[start] starting dockerd..."
mkdir -p /var/lib/docker
dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs &>/var/log/dockerd.log &

until docker info &>/dev/null; do
    sleep 0.5
done
echo "[start] dockerd ready"

# 2. Start Cortex server in the background.
echo "[start] starting cortex server..."
GITHUB_ORG=KoalbyMQP GITHUB_REPOS=Core cortex-server &>/var/log/cortex.log &

# 3. Wait until Cortex is healthy on :8080.
echo "[start] waiting for cortex on :8080..."
until curl -sf http://127.0.0.1:8080/health &>/dev/null; do
    sleep 0.5
done

echo "[start] ready"

if [ "$#" -gt 0 ]; then
    exec "$@"
fi

exec tail -f /dev/null
