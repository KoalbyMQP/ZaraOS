#!/bin/bash
set -e

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "ZaraOS Post-Image: Generating SD Card Image"
echo "═══════════════════════════════════════════════════════════════"

BOARD_DIR="${BR2_EXTERNAL_ZaraOS_PATH:-$(dirname "$(dirname "$0")")}"
GENIMAGE_CFG="${BINARIES_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

# ┌─────────────────────────────────────────────────────────────────┐
# │ VERSION                                                         │
# └─────────────────────────────────────────────────────────────────┘

VERSION_FILE="${BOARD_DIR}/VERSION"
if [ ! -f "$VERSION_FILE" ]; then
    echo "ERROR: VERSION file not found at ${VERSION_FILE}"
    exit 1
fi

# shellcheck source=/dev/null
. "$VERSION_FILE"
echo "Versions: BOARD=${BOARD_VERSION} OS=${OS_VERSION}"

# Write board-version file into BINARIES_DIR — goes onto boot FAT partition
# zaraos-update.sh reads this at runtime for OTA compatibility checks
cat > "${BINARIES_DIR}/board-version" <<EOF
BOARD_VERSION=${BOARD_VERSION}
OS_VERSION=${OS_VERSION}
EOF
echo "board-version file created"

# ┌─────────────────────────────────────────────────────────────────┐
# │ ROOTFS-B                                                        │
# └─────────────────────────────────────────────────────────────────┘

echo "Creating rootfs-b (copy of rootfs-a)..."
cp "${BINARIES_DIR}/rootfs.ext4" "${BINARIES_DIR}/rootfs-b.ext4"

# ┌─────────────────────────────────────────────────────────────────┐
# │ BOOT FILES STAGING                                              │
# └─────────────────────────────────────────────────────────────────┘

# Initial slot tracking file
echo "a" > "${BINARIES_DIR}/slot"

# Stage tryboot.txt
cp "${BOARD_DIR}/boot-configs/tryboot.txt" "${BINARIES_DIR}/tryboot.txt"

# Stage both cmdline files
cp "${BOARD_DIR}/cmdlines/cmdline_5_a.txt" "${BINARIES_DIR}/cmdline_5_a.txt"
cp "${BOARD_DIR}/cmdlines/cmdline_5_b.txt" "${BINARIES_DIR}/cmdline_5_b.txt"

# ┌─────────────────────────────────────────────────────────────────┐
# │ GENIMAGE CONFIG                                                 │
# └─────────────────────────────────────────────────────────────────┘

echo "Collecting boot files..."
FILES=()

for dtb_file in "${BINARIES_DIR}"/*.dtb; do
    [ -f "$dtb_file" ] || continue
    FILES+=( "$(basename "$dtb_file")" )
done

for fw_file in "${BINARIES_DIR}"/rpi-firmware/*; do
    [ -f "$fw_file" ] || continue
    FILES+=( "$(basename "$fw_file")" )
done

# Additional files going onto boot partition
FILES+=( "slot" "board-version" "tryboot.txt" "cmdline_5_a.txt" "cmdline_5_b.txt" )

KERNEL=$(sed -n 's/^kernel=//p' "${BINARIES_DIR}/rpi-firmware/config.txt" 2>/dev/null || echo "Image")
[ -f "${BINARIES_DIR}/${KERNEL}" ] && FILES+=( "${KERNEL}" )

BOOT_FILES=$(printf '\\t\\t\\t"%s",\\n' "${FILES[@]}")
sed "s|#BOOT_FILES#|${BOOT_FILES}|" "${BOARD_DIR}/imaging/genimage.cfg.in" > "${GENIMAGE_CFG}"
echo "Genimage config written (${#FILES[@]} boot files)"

# ┌─────────────────────────────────────────────────────────────────┐
# │ STAGE FIRMWARE TO ROOT LEVEL                                    │
# └─────────────────────────────────────────────────────────────────┘

for fw_file in "${BINARIES_DIR}"/rpi-firmware/*; do
    [ -f "$fw_file" ] || continue
    filename=$(basename "$fw_file")
    [ -f "${BINARIES_DIR}/${filename}" ] || cp "$fw_file" "${BINARIES_DIR}/${filename}"
done

if [ -d "${BINARIES_DIR}/rpi-firmware/overlays" ] && [ ! -d "${BINARIES_DIR}/overlays" ]; then
    cp -r "${BINARIES_DIR}/rpi-firmware/overlays" "${BINARIES_DIR}/overlays"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ VALIDATE                                                        │
# └─────────────────────────────────────────────────────────────────┘

for f in rootfs.ext4 rootfs-b.ext4 config.txt cmdline_5_a.txt; do
    if [ ! -f "${BINARIES_DIR}/${f}" ]; then
        echo "ERROR: missing required file: ${f}"
        exit 1
    fi
    echo "  ${f}: $(du -h "${BINARIES_DIR}/${f}" | cut -f1)"
done

# ┌─────────────────────────────────────────────────────────────────┐
# │ GENIMAGE                                                        │
# └─────────────────────────────────────────────────────────────────┘

trap 'rm -rf "${ROOTPATH_TMP}"' EXIT
ROOTPATH_TMP="$(mktemp -d)"
rm -rf "${GENIMAGE_TMP}"

genimage \
    --rootpath "${ROOTPATH_TMP}"   \
    --tmppath  "${GENIMAGE_TMP}"   \
    --inputpath  "${BINARIES_DIR}" \
    --outputpath "${BINARIES_DIR}" \
    --config "${GENIMAGE_CFG}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ ARTIFACT NAMING                                                 │
# └─────────────────────────────────────────────────────────────────┘

if [ ! -f "${BINARIES_DIR}/sdcard.img" ]; then
    echo "ERROR: sdcard.img not found after genimage"
    exit 1
fi

# Versioned flash artifact
FLASH_NAME="zaraos-board${BOARD_VERSION}-os${OS_VERSION}-sdcard.img"
cp "${BINARIES_DIR}/sdcard.img" "${BINARIES_DIR}/${FLASH_NAME}"

# Versioned OTA artifacts — rootfs only, what zaraos-update.sh downloads
OTA_NAME="zaraos-os${OS_VERSION}-rootfs.ext4"
cp "${BINARIES_DIR}/rootfs.ext4" "${BINARIES_DIR}/${OTA_NAME}"
gzip -9 -f "${BINARIES_DIR}/${OTA_NAME}"
sha256sum "${BINARIES_DIR}/${OTA_NAME}.gz" > "${BINARIES_DIR}/${OTA_NAME}.gz.sha256"

# Version manifest for OTA compatibility checks
cat > "${BINARIES_DIR}/zaraos-os${OS_VERSION}-manifest.txt" <<EOF
OS_VERSION=${OS_VERSION}
BOARD_VERSION=${BOARD_VERSION}
ROOTFS=${OTA_NAME}.gz
EOF

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "Artifacts:"
echo "  Flash:    ${FLASH_NAME}"
echo "  OTA:      ${OTA_NAME}.gz"
echo "  Manifest: zaraos-os${OS_VERSION}-manifest.txt"
echo ""
echo "Flash: sudo dd if=${BINARIES_DIR}/${FLASH_NAME} of=/dev/sdX bs=4M status=progress"
echo "═══════════════════════════════════════════════════════════════"
