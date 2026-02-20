#!/bin/sh
# zaraos-commit.sh
# Make the currently running slot permanent after a successful tryboot update.

set -e

SLOT_FILE=/boot/slot
BOOT_CFG=/boot/config.txt

if [ ! -f "$SLOT_FILE" ]; then
    echo "ERROR: $SLOT_FILE not found — is /boot mounted?"
    exit 1
fi

# Determine running slot from kernel cmdline — more reliable than any file
ROOT=$(grep -o 'root=/dev/mmcblk0p[0-9]' /proc/cmdline | head -1)
case "$ROOT" in
    *p2) CURRENT_SLOT=a; CURRENT_CMDLINE=cmdline_5_a.txt ;;
    *p3) CURRENT_SLOT=b; CURRENT_CMDLINE=cmdline_5_b.txt ;;
    *)
        echo "ERROR: cannot determine slot from cmdline: $ROOT"
        exit 1
        ;;
esac

COMMITTED=$(cat "$SLOT_FILE")

if [ "$CURRENT_SLOT" = "$COMMITTED" ]; then
    echo "Slot ${CURRENT_SLOT} already committed — nothing to do."
    exit 0
fi

echo "Committing slot ${CURRENT_SLOT} (was: ${COMMITTED})..."

# Make cmdline permanent in config.txt
sed -i "s/cmdline=cmdline_5_.\.txt/cmdline=${CURRENT_CMDLINE}/" "$BOOT_CFG"

# Update slot file
echo "$CURRENT_SLOT" > "$SLOT_FILE"

# Log the committed versions
# shellcheck source=/dev/null
. /etc/zaraos-release
echo "board=${BOARD_VERSION} os=${OS_VERSION}" > /boot/committed-version
echo "committed_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /boot/committed-version

sync

echo "Committed. Active slot: ${CURRENT_SLOT}"
echo "  board=${BOARD_VERSION} os=${OS_VERSION}"
echo "Slot ${COMMITTED} remains available as target for next OTA."
