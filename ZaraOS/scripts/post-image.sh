#!/bin/bash
# ===================================================================
# ZaraOS Post-Image Script - FIXED VERSION
# ===================================================================
# This script runs after all filesystem images are created and
# generates the final SD card image using genimage. It handles
# the complex process of combining boot and root filesystems.
#
# Documentation:
# - Buildroot Post-Image: https://buildroot.org/downloads/manual/manual.html#rootfs-custom
# - Genimage Tool: https://github.com/pengutronix/genimage
# - Pi Boot Process: https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#boot-sequence
#
# Environment:
# - BINARIES_DIR: Contains all built images and firmware
# - BUILD_DIR: Temporary build directory
# - Called after rootfs.ext4 and boot files are ready
# ===================================================================

set -e  # Exit on any error

# ┌─────────────────────────────────────────────────────────────────┐
# │ ENVIRONMENT SETUP                                               │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "ZaraOS Post-Image: Generating SD Card Image"
echo "═══════════════════════════════════════════════════════════════"

# Use hardcoded paths for ZaraOS
BOARD_DIR="/workspace/ZaraOS"
GENIMAGE_CFG="${BINARIES_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

echo "ZaraOS directory: ${BOARD_DIR}"
echo "Genimage config: ${GENIMAGE_CFG}"
echo "Temporary directory: ${GENIMAGE_TMP}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ GENIMAGE CONFIGURATION GENERATION                               │
# └─────────────────────────────────────────────────────────────────┘

# Always generate genimage configuration from template
echo ""
echo "Generating genimage config from template"

FILES=()

echo "Collecting boot files:"

# Collect all device tree files
for dtb_file in "${BINARIES_DIR}"/*.dtb; do
    if [ -f "$dtb_file" ]; then
        filename=$(basename "$dtb_file")
        FILES+=( "$filename" )
        echo "   Device Tree: $filename"
    fi
done

# Collect all Raspberry Pi firmware files - Use basename only
for fw_file in "${BINARIES_DIR}"/rpi-firmware/*; do
    if [ -f "$fw_file" ]; then
        filename=$(basename "$fw_file")
        FILES+=( "$filename" )
        echo "   Firmware: $filename"
    fi
done

# Handle overlays directory specially - we need to preserve directory structure
if [ -d "${BINARIES_DIR}/rpi-firmware/overlays" ]; then
    echo "   Overlays directory: overlays/ ($(ls "${BINARIES_DIR}/rpi-firmware/overlays" | wc -l) files)"
fi

# Determine kernel image name from config.txt
KERNEL=$(sed -n 's/^kernel=//p' "${BINARIES_DIR}/rpi-firmware/config.txt" 2>/dev/null || echo "zImage")
if [ -f "${BINARIES_DIR}/${KERNEL}" ]; then
    FILES+=( "${KERNEL}" )
    echo "   Kernel: ${KERNEL}"
else
    echo "   Warning: Kernel file '${KERNEL}' not found"
fi

# Generate the boot files list for genimage template
BOOT_FILES=$(printf '\\t\\t\\t"%s",\\n' "${FILES[@]}")

echo ""
echo "Creating genimage configuration"

# Substitute the boot files list in the template
sed "s|#BOOT_FILES#|${BOOT_FILES}|" "${BOARD_DIR}/imaging/genimage.cfg.in" \
    > "${GENIMAGE_CFG}"

echo "Genimage configuration created with ${#FILES[@]} boot files"

# ┌─────────────────────────────────────────────────────────────────┐
# │ COPY FIRMWARE FILES TO ROOT LEVEL FOR GENIMAGE                  │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "Copying firmware files to root level for genimage"

# Copy firmware files from rpi-firmware subdirectory to root level
# This ensures genimage can find them without subdirectory paths
for fw_file in "${BINARIES_DIR}"/rpi-firmware/*; do
    if [ -f "$fw_file" ]; then
        filename=$(basename "$fw_file")
        if [ ! -f "${BINARIES_DIR}/${filename}" ]; then
            cp "$fw_file" "${BINARIES_DIR}/${filename}"
            echo "   Copied: $filename"
        fi
    fi
done

# Copy overlays directory maintaining directory structure
if [ -d "${BINARIES_DIR}/rpi-firmware/overlays" ]; then
    if [ ! -d "${BINARIES_DIR}/overlays" ]; then
        cp -r "${BINARIES_DIR}/rpi-firmware/overlays" "${BINARIES_DIR}/overlays"
        overlay_count=$(find "${BINARIES_DIR}/overlays" -type f | wc -l)
        echo "   Copied: overlays/ directory with ${overlay_count} files"
    fi
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ IMAGE VALIDATION                                                │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "Validating required images"

# Check that essential files exist
REQUIRED_FILES=(
    "${BINARIES_DIR}/rootfs.ext4"
    "${BINARIES_DIR}/config.txt"
    "${BINARIES_DIR}/cmdline.txt"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        size=$(du -h "$file" | cut -f1)
        echo "   $(basename "$file"): $size"
    else
        echo "   Missing: $(basename "$file")"
        exit 1
    fi
done

# Check overlays directory
if [ -d "${BINARIES_DIR}/overlays" ]; then
    overlay_count=$(find "${BINARIES_DIR}/overlays" -type f | wc -l)
    echo "   overlays/: ${overlay_count} files"
else
    echo "   Warning: overlays/ directory not found"
fi

# Display boot partition contents summary
echo ""
echo "Boot partition will contain:"
cat "${GENIMAGE_CFG}" | grep -A 10 "files = {" | grep '"' | sed 's/.*"\(.*\)".*/   • \1/'

# Show overlays directory information
if [ -d "${BINARIES_DIR}/overlays" ]; then
    overlay_count=$(find "${BINARIES_DIR}/overlays" -type f | wc -l)
    echo "   • overlays/ directory (${overlay_count} overlay files)"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ GENIMAGE EXECUTION                                              │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "Running genimage to create SD card image"

# Create a temporary empty rootpath
# genimage makes a full copy of rootpath, so we use empty directory
# to avoid copying the entire target filesystem unnecessarily
trap 'rm -rf "${ROOTPATH_TMP}"' EXIT
ROOTPATH_TMP="$(mktemp -d)"

# Clean previous genimage temporary files
rm -rf "${GENIMAGE_TMP}"

echo "Cleaned temporary directories"
echo "Generating image"

# Run genimage with proper parameters
genimage \
    --rootpath "${ROOTPATH_TMP}"   \
    --tmppath "${GENIMAGE_TMP}"    \
    --inputpath "${BINARIES_DIR}"  \
    --outputpath "${BINARIES_DIR}" \
    --config "${GENIMAGE_CFG}"

GENIMAGE_EXIT_CODE=$?

# ┌─────────────────────────────────────────────────────────────────┐
# │ RESULT VALIDATION AND REPORTING                                 │
# └─────────────────────────────────────────────────────────────────┘

echo ""
if [ ${GENIMAGE_EXIT_CODE} -eq 0 ]; then
    echo "SD card image generation completed successfully"
    
    # Display final image information
    if [ -f "${BINARIES_DIR}/sdcard.img" ]; then
        IMAGE_SIZE=$(du -h "${BINARIES_DIR}/sdcard.img" | cut -f1)
        IMAGE_SIZE_BYTES=$(stat -c%s "${BINARIES_DIR}/sdcard.img" 2>/dev/null || echo "unknown")
        
        echo ""
        echo "Final Image Information:"
        echo "   Location: ${BINARIES_DIR}/sdcard.img"
        echo "   Size: ${IMAGE_SIZE} (${IMAGE_SIZE_BYTES} bytes)"
        echo "   Target: Raspberry Pi 5"
        echo ""
        echo "Ready to flash! Use:"
        echo "   sudo dd if=${BINARIES_DIR}/sdcard.img of=/dev/sdX bs=4M status=progress"
        echo "   (Replace /dev/sdX with your SD card device)"
    else
        echo "Warning: sdcard.img not found after generation"
        GENIMAGE_EXIT_CODE=1
    fi
else
    echo "Genimage failed with exit code: ${GENIMAGE_EXIT_CODE}"
    echo ""
    echo "Troubleshooting tips:"
    echo "   • Check ${GENIMAGE_TMP} for detailed logs"
    echo "   • Verify all required files exist in ${BINARIES_DIR}"
    echo "   • Ensure sufficient disk space available"
    echo "   • Review genimage configuration: ${GENIMAGE_CFG}"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ CLEANUP AND EXIT                                                │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "═══════════════════════════════════════════════════════════════"
if [ ${GENIMAGE_EXIT_CODE} -eq 0 ]; then
    echo "ZaraOS SD card image ready for deployment"
else
    echo "ZaraOS image generation failed"
fi
echo "═══════════════════════════════════════════════════════════════"
echo ""

exit ${GENIMAGE_EXIT_CODE}
