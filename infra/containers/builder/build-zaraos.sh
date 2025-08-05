#!/bin/bash
set -e

echo "Building ZaraOS"

# For GitHub Actions runner - we're in the repo root, ZaraOS subdir contains the external tree
WORKSPACE_PATH="$(pwd)"
EXTERNAL_PATH="$(pwd)/ZaraOS"

echo "Using workspace: $WORKSPACE_PATH"
echo "Using external path: $EXTERNAL_PATH"

BUILD_DIR="/tmp/zaraos-build"
DL_DIR="/tmp/zaraos-dl"
mkdir -p "$BUILD_DIR" "$DL_DIR"

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="$EXTERNAL_PATH" \
    zaraos_pi5_defconfig

make -C /opt/buildroot \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="$EXTERNAL_PATH" \
    -j2

echo "Copying artifacts..."
mkdir -p "$WORKSPACE_PATH/output"

# Copy all images (final artifacts)
if [ -d "$BUILD_DIR/images" ]; then
    echo "Copying final images..."
    cp -r "$BUILD_DIR/images"/* "$WORKSPACE_PATH/output/"
fi

echo "Build complete!"
echo "Final images available in: $WORKSPACE_PATH/output/"

