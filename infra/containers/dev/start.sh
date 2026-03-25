#!/usr/bin/env bash
# ZaraOS Development Environment Bootstrap

LOG_DIR="/var/log"
mkdir -p "$LOG_DIR"
touch "$LOG_DIR/dockerd.log" "$LOG_DIR/cortex.log"

echo "[ZaraOS] Starting development services..."

# --- 1. Docker-in-Docker (DinD) ---
echo "[1/3] Initializing Docker daemon (Driver: vfs)..."
if ! pgrep -x "dockerd" > /dev/null; then
    mkdir -p /var/lib/docker
    # Storage driver vfs is used for nested cloud environments (us-east-1)
    dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs > "$LOG_DIR/dockerd.log" 2>&1 &
fi

# Wait for Docker with a 30s timeout
MAX_RETRIES=60
COUNT=0
until docker info >/dev/null 2>&1 || [ $COUNT -eq $MAX_RETRIES ]; do
    sleep 0.5
    ((COUNT++))
done

if [ $COUNT -eq $MAX_RETRIES ]; then
    echo "ERROR: Docker failed to start. Check $LOG_DIR/dockerd.log"
    exit 1
fi
echo "Docker is ready."

# --- 2. Cortex Server ---
echo "[2/3] Launching Cortex server (Org: KoalbyMQP)..."
GITHUB_ORG=KoalbyMQP GITHUB_REPOS=Core cortex-server > "$LOG_DIR/cortex.log" 2>&1 &

# --- 3. Health Check ---
echo "[3/3] Waiting for Cortex health check on :8080..."
COUNT=0
until curl -sf http://127.0.0.1:8080/health >/dev/null 2>&1 || [ $COUNT -eq $MAX_RETRIES ]; do
    sleep 0.5
    ((COUNT++))

    # Immediate exit if process crashes during boot
    if ! pgrep -f "cortex-server" > /dev/null; then
        echo "ERROR: Cortex server crashed. Check $LOG_DIR/cortex.log"
        exit 1
    fi
done

if [ $COUNT -eq $MAX_RETRIES ]; then
    echo "ERROR: Cortex health check timed out."
    exit 1
fi

echo "[ZaraOS] Environment is READY."

# --- 4. Lifecycle Management ---
if [ "$#" -gt 0 ]; then
    exec "$@"
fi

# Exit if in DevPod/DevContainer to allow VS Code to finalize attachment
if [ -n "${DEVPOD:-}" ] || [ -n "${REMOTE_CONTAINERS:-}" ] || [ -n "${CODESPACES:-}" ]; then
    echo "[start] Setup complete, returning control to environment."
    exit 0
fi

# Keep container alive for manual 'docker run'
echo "[start] No Dev Environment detected, idling with tail..."
exec tail -f /dev/null
