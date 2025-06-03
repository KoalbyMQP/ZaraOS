#!/bin/bash
set -e

echo "ZaraOS Pure Buildroot Environment"
echo "================================="

if [ ! -f "flake.nix" ]; then
    echo "Error: Not in ZaraOS workspace"
    exit 1
fi

if [ ! -d "ZaraOS/buildroot" ]; then
    echo "Linking buildroot..."
    mkdir -p ZaraOS
    ln -sf /home/buildroot/buildroot ZaraOS/buildroot
fi

CONFIG_PATH="ZaraOS/external/configs/zaraos_pi5_defconfig"
OUTPUT_DIR="/workspace/ZaraOS/output"
DL_DIR="/workspace/ZaraOS/dl"

if [ ! -f "$CONFIG_PATH" ]; then
    echo "Error: Config not found: $CONFIG_PATH"
    exit 1
fi

echo "Setting up build environment..."
mkdir -p "$OUTPUT_DIR" "$DL_DIR"

export BR2_EXTERNAL="/workspace/ZaraOS/external"
MAKE_FLAGS="O=$OUTPUT_DIR BR2_DL_DIR=$DL_DIR"

# Optimize build for container environment
export MAKEFLAGS="-j$(nproc)"

echo "Configuring ZaraOS..."
make -C ZaraOS/buildroot $MAKE_FLAGS zaraos_pi5_defconfig

echo "Building ZaraOS..."
echo "This will take a while - grab some coffee ☕"
make -C ZaraOS/buildroot $MAKE_FLAGS

echo ""
echo "🎉 Build complete!"
echo "Output images: $OUTPUT_DIR/images/"
echo ""