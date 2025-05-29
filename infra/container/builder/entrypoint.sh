#!/usr/bin/env bash
set -e

echo "ZaraOS Container Build Environment"

# Verify we're in the right directory
if [ ! -f "flake.nix" ]; then
    echo "Error: flake.nix not found. Make sure source is mounted at /workspace"
    exit 1
fi

# Enter the build environment and run the actual build
# TODO: This needs to be a lot mor esoffisticated
nix develop .#build --command bash -c '
    set -e
    
    CONFIG_PATH="ZaraOS/external/configs/zaraos_pi5_defconfig"
    OUTPUT_DIR="/workspace/ZaraOS/output"
    DL_DIR="/workspace/ZaraOS/dl"
    
    echo "Building ZaraOS"
    
    if [ ! -f "$CONFIG_PATH" ]; then
        echo "Error: Config not found: $CONFIG_PATH"
        exit 1
    fi
    
    mkdir -p "$OUTPUT_DIR" "$DL_DIR"
    
    export BR2_EXTERNAL="/workspace/ZaraOS/external"
    MAKE_FLAGS="O=$OUTPUT_DIR BR2_DL_DIR=$DL_DIR"
    
    echo "Configuring"
    make -C ZaraOS/buildroot $MAKE_FLAGS zaraos_pi5_defconfig
    
    echo "Building"
    make -C ZaraOS/buildroot $MAKE_FLAGS
    
    echo "Build complete: $OUTPUT_DIR/images/"
'