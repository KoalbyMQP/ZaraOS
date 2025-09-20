#!/bin/bash
set -e

echo "Building ZaraOS"

WORKSPACE_PATH="${WORKSPACE_PATH:-$(pwd)}"
EXTERNAL_PATH="${EXTERNAL_PATH:-${WORKSPACE_PATH}/ZaraOS}"
BUILD_DIR="${BUILD_DIR:-/tmp/zaraos-build}"
DL_DIR="${DL_DIR:-/tmp/zaraos-dl}"
OUTPUT_DIR="${OUTPUT_DIR:-${WORKSPACE_PATH}/output}"
DEFCONFIG="${DEFCONFIG:-zaraos_pi5_defconfig}"
JOBS="${JOBS:-$(nproc)}"

echo "Workspace: $WORKSPACE_PATH"
echo "External: $EXTERNAL_PATH"
echo "Build dir: $BUILD_DIR"
echo "Jobs: $JOBS"

if [[ ! -d "/opt/buildroot" ]]; then
    echo "ERROR: Buildroot not found"
    exit 1
fi

if [[ ! -d "$EXTERNAL_PATH" ]]; then
    echo "ERROR: ZaraOS external tree not found"
    exit 1
fi

rm -rf "$BUILD_DIR" "$DL_DIR" "$OUTPUT_DIR" || true
mkdir -p "$BUILD_DIR" "$DL_DIR" "$OUTPUT_DIR"

echo "Configuring build..."
make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="$EXTERNAL_PATH" \
    "$DEFCONFIG"

echo "Building..."
make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="$EXTERNAL_PATH" \
    -j"$JOBS"

echo "Copying artifacts..."
if [ -d "$BUILD_DIR/images" ]; then
    cp -r "$BUILD_DIR/images"/* "$OUTPUT_DIR/"
fi

echo "Build complete"
ls -la "$OUTPUT_DIR/"
