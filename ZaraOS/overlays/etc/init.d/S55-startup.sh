#!/bin/sh
# ===================================================================
# ZaraOS First-Boot Setup Wizard
# ===================================================================
# Runs the containerized setup UI on the DRM framebuffer.
# Only executes once — checks /data/.setup-done flag.
#
# The container image is pre-baked at /opt/startup-image.tar
# (placed there by CI before the Buildroot build).
#
# The entire startup flow runs in the BACKGROUND so the rest of
# the boot (SSH, cortex, etc.) is not blocked by the slow
# nerdctl load on emulated arm64.
#
# Requires: containerd (started in rcS), nerdctl, /data mounted (S01)
# ===================================================================

SETUP_FLAG="/data/.setup-done"
IMAGE_TAR="/opt/startup-image.tar"
IMAGE_NAME="ghcr.io/koalbymqp/zaraos-startup:latest"
RUNNING_FLAG="/tmp/.startup-app-running"
LOG="/tmp/startup-app.log"

log() {
    echo "[S55] $1"
    echo "[$(date '+%H:%M:%S')] $1" >> "$LOG"
}

run_startup() {
    log "Starting ZaraOS first-boot setup wizard..."

    # ── Kill psplash (safety — rcS should have quit it) ─────
    echo "QUIT" > /run/psplash_fifo 2>/dev/null || true
    killall psplash 2>/dev/null || true
    sleep 0.5

    # ── Signal that the startup app owns the display ─────────
    touch "$RUNNING_FLAG"

    # ── Diagnostics: log DRM state ───────────────────────────
    log "=== PRE-SETUP DIAGNOSTICS ==="
    log "DRI devices: $(ls /dev/dri/ 2>/dev/null || echo 'none')"
    log "Framebuffer: $(ls /dev/fb* 2>/dev/null || echo 'none')"
    for vtcon in /sys/class/vtconsole/vtcon*/bind; do
        [ -f "$vtcon" ] && log "  $vtcon = $(cat "$vtcon") ($(cat "${vtcon%/bind}/name" 2>/dev/null))"
    done
    log "Active VT: $(cat /sys/class/tty/tty0/active 2>/dev/null || echo unknown)"
    log "=== END DIAGNOSTICS ==="

    # ── Release DRM for KMSDRM backend ──────────────────────
    pkill -f "getty.*tty1" 2>/dev/null || true
    pkill -f "startup-gate" 2>/dev/null || true
    sleep 0.3

    # Unbind fbcon to release DRM master
    for vtcon in /sys/class/vtconsole/vtcon*/bind; do
        [ -f "$vtcon" ] && echo 0 > "$vtcon" 2>/dev/null || true
    done
    chvt 7 2>/dev/null || true
    sleep 0.3

    log "Post-unbind VT state:"
    for vtcon in /sys/class/vtconsole/vtcon*/bind; do
        [ -f "$vtcon" ] && log "  $vtcon = $(cat "$vtcon")"
    done

    # ── Import the pre-baked container image ─────────────────
    if [ -f "$IMAGE_TAR" ]; then
        log "Importing container image ($(du -h "$IMAGE_TAR" | cut -f1))..."

        # Wait for containerd to be fully ready (socket + snapshotter initialized)
        WAIT=60
        while [ ! -S /run/containerd/containerd.sock ] && [ $WAIT -gt 0 ]; do
            log "  Waiting for containerd socket... ($WAIT)"
            sleep 1
            WAIT=$((WAIT - 1))
        done
        if [ ! -S /run/containerd/containerd.sock ]; then
            log "FAILED: containerd socket never appeared"
            rm -f "$RUNNING_FLAG"
            return 1
        fi
        log "  containerd socket ready, waiting for snapshotter init..."

        # containerd needs a moment to initialize its snapshotters after
        # the socket appears — especially on a fresh data partition.
        WAIT=30
        while [ ! -f /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/metadata.db ] && [ $WAIT -gt 0 ]; do
            sleep 1
            WAIT=$((WAIT - 1))
        done
        # Even if the file doesn't appear, give containerd a grace period
        sleep 3
        log "  containerd fully ready."

        LOAD_OUT=$(nerdctl load < "$IMAGE_TAR" 2>&1)
        RC=$?
        log "  nerdctl load output: $LOAD_OUT"
        if [ $RC -ne 0 ]; then
            log "FAILED: nerdctl load returned $RC"
            log "  containerd running: $(pgrep containerd && echo yes || echo no)"
            log "  socket: $(ls -la /run/containerd/containerd.sock 2>&1)"
            log "  images: $(nerdctl images 2>&1)"
            rm -f "$RUNNING_FLAG"
            return 1
        fi
        log "Container image imported."
    else
        log "No $IMAGE_TAR, checking if image exists in containerd..."
        nerdctl image inspect "$IMAGE_NAME" >> "$LOG" 2>&1 || {
            log "FAILED: No image tar and image not in containerd"
            rm -f "$RUNNING_FLAG"
            return 1
        }
    fi

    # ── WiFi prep ────────────────────────────────────────────
    mkdir -p /var/run/wpa_supplicant
    if [ -e /sys/class/net/wlan0 ]; then
        wpa_supplicant -B -i wlan0 -C /var/run/wpa_supplicant 2>/dev/null || true
    fi
    touch /etc/wpa_supplicant.conf 2>/dev/null || true

    # ── Run the setup wizard container ───────────────────────
    log "Launching container..."
    log "  DRI: $(ls /dev/dri/ 2>/dev/null || echo none)"

    CONTAINER_LOG="/tmp/container-output.log"
    nerdctl run --rm \
        --privileged \
        --net=host \
        --pid=host \
        -v /dev:/dev \
        -v /sys:/sys:ro \
        -v /data:/data \
        -v /etc/hostname:/etc/hostname \
        -v /etc/wpa_supplicant.conf:/etc/wpa_supplicant.conf \
        -v /var/run/wpa_supplicant:/var/run/wpa_supplicant \
        -v /run/containerd/containerd.sock:/run/containerd/containerd.sock \
        -e SDL_VIDEODRIVER=kmsdrm \
        -e SDL_AUDIODRIVER=dummy \
        "$IMAGE_NAME" \
        > "$CONTAINER_LOG" 2>&1

    RC=$?
    log "Container exited with code $RC"
    log "=== CONTAINER OUTPUT (nerdctl stdout) ==="
    cat "$CONTAINER_LOG" >> "$LOG" 2>/dev/null
    log "=== CONTAINER LOG (from /data) ==="
    cat /data/startup-output.log >> "$LOG" 2>/dev/null || log "  (no /data/startup-output.log)"
    log "=== END CONTAINER OUTPUT ==="

    # ── Clean up ─────────────────────────────────────────────
    rm -f "$RUNNING_FLAG"

    # Re-bind fbcon and switch back
    for vtcon in /sys/class/vtconsole/vtcon*/bind; do
        [ -f "$vtcon" ] && echo 1 > "$vtcon" 2>/dev/null || true
    done
    chvt 1 2>/dev/null || true

    # Delete the tar to reclaim rootfs space
    if [ -f "$IMAGE_TAR" ] && [ -f "$SETUP_FLAG" ]; then
        log "Removing pre-baked image tar."
        rm -f "$IMAGE_TAR"
    fi

    log "S55-startup finished."
}

case "$1" in
    start)
        # Fast path: skip if setup already done
        if [ -f "$SETUP_FLAG" ]; then
            echo "Setup already done, skipping startup wizard."
            exit 0
        fi

        # Run everything in the background so the boot sequence
        # continues immediately.  Other services (SSH, cortex)
        # don't wait for the slow nerdctl load.
        echo "Launching setup wizard in background..."
        run_startup < /dev/null &
        ;;
    stop)
        if [ -f "$RUNNING_FLAG" ]; then
            nerdctl kill "$(nerdctl ps -q --filter ancestor="$IMAGE_NAME")" 2>/dev/null || true
            rm -f "$RUNNING_FLAG"
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop}"
        exit 1
        ;;
esac
