#!/bin/bash
set -e

echo "Building ZaraOS"

BUILD_DIR="/tmp/zaraos-build"
DL_DIR="/tmp/zaraos-dl"
mkdir -p "$BUILD_DIR" "$DL_DIR"

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="/workspace/ZaraOS/external" \
    zaraos_pi5_defconfig

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="/workspace/ZaraOS/external" \
    -j2

echo "Copying artifacts..."
mkdir -p /workspace/output
cp -r "$BUILD_DIR/images"/* /workspace/output/

echo "Build complete!"