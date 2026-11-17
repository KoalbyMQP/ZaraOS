#!/bin/sh
# ===================================================================
# zaraos-roudi.sh - launch the iceoryx RouDi daemon for ZaraOS
# ===================================================================
# Started (and stopped) on demand by cortex; not an init script.
#
# iceoryx 2.0 hardcodes its unix-domain sockets under /tmp/ (RouDi
# listens on /tmp/roudi, every client creates /tmp/<runtime-name>), so
# RouDi and all shared-memory app containers must see the same /tmp.
# Rather than sharing the host's whole /tmp, RouDi runs in a private
# mount namespace where /tmp is a bind of $IOX_DIR, and cortex mounts
# the same $IOX_DIR at /tmp inside each shared-memory container.
#
# unshare/sh/iox-roudi all exec in place, so the PID cortex started
# is RouDi itself and SIGTERM shuts it down cleanly.
# ===================================================================
set -e

IOX_DIR="${IOX_DIR:-/tmp/iceoryx}"
ROUDI_CONFIG="${ROUDI_CONFIG:-/etc/iceoryx/roudi_config.toml}"

mkdir -p "$IOX_DIR"
chmod 1777 "$IOX_DIR"

exec unshare -m --propagation private sh -c \
    "mount --bind '$IOX_DIR' /tmp && exec iox-roudi -c '$ROUDI_CONFIG'"
