#!/bin/bash
set -e

echo "Building ZaraOS"

# Debug: Check if files exist
echo "=== DEBUG: Checking ZaraOS structure ==="
ls -la /workspace/ZaraOS/
echo "=== DEBUG: Checking patches directory ==="
ls -la /workspace/ZaraOS/patches/ || echo "patches directory not found!"
echo "=== DEBUG: Checking linux patches ==="
ls -la /workspace/ZaraOS/patches/linux/ || echo "linux patches not found!"
echo "=== DEBUG: END ==="

BUILD_DIR="/tmp/zaraos-build"
DL_DIR="/tmp/zaraos-dl"
mkdir -p "$BUILD_DIR" "$DL_DIR"

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="/workspace/ZaraOS" \
    zaraos_pi5_defconfig

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="/workspace/ZaraOS" \
    -j2

echo "Copying artifacts..."
mkdir -p /workspace/output
cp -r "$BUILD_DIR/images"/* /workspace/output/

echo "Build complete!"