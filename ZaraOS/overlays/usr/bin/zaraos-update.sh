#!/bin/sh
# zaraos-update.sh <manifest_url>
#
# Downloads the version manifest first, checks BOARD_VERSION compatibility,
# then downloads and applies the rootfs OTA update.
#
# Manifest format (3 lines):
#   OS_VERSION=0.1.0
#   BOARD_VERSION=0.0a
#   ROOTFS=zaraos-os0.1.0-rootfs.ext4.gz
#
# The manifest URL base is also used to resolve the rootfs and sha256 URLs.

set -e

DISK=/dev/mmcblk0
SLOT_A=${DISK}p2
SLOT_B=${DISK}p3
STAGING=/data/update-staging
SLOT_FILE=/boot/slot
BOARD_VERSION_FILE=/boot/board-version

usage() {
    echo "Usage: $0 <manifest_url>"
    echo "  e.g. $0 https://github.com/KoalbyMQP/ZaraOS/releases/download/os-v0.1.0/zaraos-os0.1.0-manifest.txt"
    exit 1
}

[ $# -eq 1 ] || usage
MANIFEST_URL="$1"
BASE_URL=$(dirname "$MANIFEST_URL")

# ┌─────────────────────────────────────────────────────────────────┐
# │ PRE-FLIGHT CHECKS                                               │
# └─────────────────────────────────────────────────────────────────┘

if [ ! -f "$SLOT_FILE" ]; then
    echo "ERROR: $SLOT_FILE not found — is /boot mounted?"
    exit 1
fi

if [ ! -f "$BOARD_VERSION_FILE" ]; then
    echo "ERROR: $BOARD_VERSION_FILE not found — is /boot mounted?"
    exit 1
fi

# shellcheck source=/dev/null
. "$BOARD_VERSION_FILE"
CURRENT_BOARD="$BOARD_VERSION"
CURRENT_OS="$OS_VERSION"

echo "Current: board=${CURRENT_BOARD} os=${CURRENT_OS}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ MANIFEST                                                        │
# └─────────────────────────────────────────────────────────────────┘

mkdir -p "$STAGING"
MANIFEST="$STAGING/manifest.txt"

echo "Fetching manifest..."
wget -q -O "$MANIFEST" "$MANIFEST_URL"

# shellcheck source=/dev/null
. "$MANIFEST"
# Now have: OS_VERSION, BOARD_VERSION, ROOTFS from manifest
REQUIRED_BOARD="$BOARD_VERSION"
TARGET_OS="$OS_VERSION"
ROOTFS_FILENAME="$ROOTFS"

echo "Update: board=${REQUIRED_BOARD} os=${TARGET_OS}"

# Board version compatibility check — if it doesn't match, OTA can't help
if [ "$CURRENT_BOARD" != "$REQUIRED_BOARD" ]; then
    echo ""
    echo "ERROR: board version mismatch."
    echo "  Current:  ${CURRENT_BOARD}"
    echo "  Required: ${REQUIRED_BOARD}"
    echo "  This update requires a full reflash."
    rm -f "$MANIFEST"
    exit 1
fi

if [ "$CURRENT_OS" = "$TARGET_OS" ]; then
    echo "Already on os=${TARGET_OS}, nothing to do."
    rm -f "$MANIFEST"
    exit 0
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ SLOT SELECTION                                                  │
# └─────────────────────────────────────────────────────────────────┘

ACTIVE=$(cat "$SLOT_FILE")
case "$ACTIVE" in
    a) INACTIVE_SLOT=$SLOT_B; INACTIVE_CMDLINE=cmdline_5_b.txt ;;
    b) INACTIVE_SLOT=$SLOT_A; INACTIVE_CMDLINE=cmdline_5_a.txt ;;
    *)
        echo "ERROR: invalid slot '${ACTIVE}' in $SLOT_FILE"
        exit 1
        ;;
esac

echo "Active slot: ${ACTIVE} — writing to inactive: ${INACTIVE_SLOT}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ DOWNLOAD & VERIFY                                               │
# └─────────────────────────────────────────────────────────────────┘

ROOTFS_GZ="$STAGING/$ROOTFS_FILENAME"
SHA256_FILE="$STAGING/${ROOTFS_FILENAME}.sha256"

echo "Downloading rootfs..."
wget -O "$ROOTFS_GZ" "${BASE_URL}/${ROOTFS_FILENAME}"

echo "Downloading checksum..."
wget -O "$SHA256_FILE" "${BASE_URL}/${ROOTFS_FILENAME}.sha256"

echo "Verifying checksum..."
EXPECTED=$(awk '{print $1}' "$SHA256_FILE")
ACTUAL=$(sha256sum "$ROOTFS_GZ" | awk '{print $1}')
if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "ERROR: checksum mismatch — aborting"
    echo "  expected: $EXPECTED"
    echo "  actual:   $ACTUAL"
    rm -f "$ROOTFS_GZ" "$SHA256_FILE" "$MANIFEST"
    exit 1
fi
echo "Checksum OK."

# ┌─────────────────────────────────────────────────────────────────┐
# │ WRITE INACTIVE SLOT                                             │
# └─────────────────────────────────────────────────────────────────┘

echo "Writing to ${INACTIVE_SLOT}..."
gunzip -c "$ROOTFS_GZ" | dd of="$INACTIVE_SLOT" bs=4M status=progress
sync

rm -f "$ROOTFS_GZ" "$SHA256_FILE" "$MANIFEST"

# ┌─────────────────────────────────────────────────────────────────┐
# │ TRYBOOT                                                         │
# └─────────────────────────────────────────────────────────────────┘

echo "Configuring tryboot for slot ${INACTIVE_CMDLINE}..."
sed "s/cmdline=cmdline_5_.\.txt/cmdline=${INACTIVE_CMDLINE}/" /boot/config.txt > /boot/tryboot.txt

echo ""
echo "Update ready: os ${CURRENT_OS} → ${TARGET_OS}"
echo "Rebooting via tryboot. Run 'zaraos-commit' after successful boot."
echo "Pi will auto-fallback to slot ${ACTIVE} if new slot fails."
sleep 2
reboot "0xc0"
