#!/bin/sh
# ===================================================================
# TTY1 Startup Gate
# ===================================================================
# Replaces direct getty on tty1 in inittab. Defers the login shell
# until the first-boot setup wizard finishes (it needs exclusive
# DRM access). On subsequent boots, immediately execs getty.
#
# BusyBox inittab respawns this script, so it acts as a persistent
# guard — when getty eventually exits, this runs again and re-execs.
# ===================================================================

SETUP_FLAG="/data/.setup-done"
RUNNING_FLAG="/tmp/.startup-app-running"

# ── Fast path: setup already done — go straight to getty ─────────
if [ -f "$SETUP_FLAG" ]; then
    exec /sbin/getty -n -L -l /usr/bin/autologin.sh tty1 0 vt100
fi

# ── First boot: wait for the startup wizard to finish ────────────

# Wait for S55-startup.sh to either start or signal completion.
# Timeout after 120s in case S55 never runs (e.g. image missing).
timeout=120
while [ ! -f "$SETUP_FLAG" ] && [ ! -f "$RUNNING_FLAG" ] && [ $timeout -gt 0 ]; do
    sleep 1
    timeout=$((timeout - 1))
done

# If the startup app is running, wait for it to finish
while [ -f "$RUNNING_FLAG" ]; do
    sleep 1
done

# Setup is done (or timed out) — start normal getty
exec /sbin/getty -n -L -l /usr/bin/autologin.sh tty1 0 vt100
