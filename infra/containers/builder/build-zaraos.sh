#!/bin/bash
set -e

echo "Building ZaraOS"

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

# Copy all images (final artifacts)
if [ -d "$BUILD_DIR/images" ]; then
    echo "Copying final images..."
    cp -r "$BUILD_DIR/images"/* /workspace/output/
fi

# # Copy entire build results for complete build artifacts
# echo "Copying complete build results..."
# mkdir -p /workspace/output/build-results

# # Copy the entire build directory structure
# if [ -d "$BUILD_DIR" ]; then
#     echo "Copying build directory contents..."
#     # Copy all build directory contents including downloads
#     rsync -av "$BUILD_DIR/" /workspace/output/build-results/
# fi

# # Also copy the download directory separately for easier access
# if [ -d "$DL_DIR" ]; then
#     echo "Copying download cache..."
#     mkdir -p /workspace/output/downloads
#     rsync -av "$DL_DIR/" /workspace/output/downloads/
# fi

# echo "Copying logs and debug info..."
# mkdir -p /workspace/output/logs

# # Copy main build log if it exists
# if [ -f "$BUILD_DIR/build.log" ]; then
#     cp "$BUILD_DIR/build.log" /workspace/output/logs/
# fi

# # Copy Linux kernel build logs and config
# LINUX_BUILD_DIR=$(find "$BUILD_DIR/build" -maxdepth 1 -name "linux-*" -type d | head -1)
# if [ -n "$LINUX_BUILD_DIR" ]; then
#     echo "Found Linux build directory: $LINUX_BUILD_DIR"
    
#     # Copy kernel config that was actually used
#     if [ -f "$LINUX_BUILD_DIR/.config" ]; then
#         cp "$LINUX_BUILD_DIR/.config" /workspace/output/logs/kernel.config
#         echo "Copied kernel config"
#     fi
    
#     # Copy kernel build log if it exists
#     if [ -f "$LINUX_BUILD_DIR/build.log" ]; then
#         cp "$LINUX_BUILD_DIR/build.log" /workspace/output/logs/kernel-build.log
#         echo "Copied kernel build log"
#     fi
    
#     # Copy fragment application info
#     if [ -f "$LINUX_BUILD_DIR/.config.old" ]; then
#         cp "$LINUX_BUILD_DIR/.config.old" /workspace/output/logs/kernel.config.old
#         echo "Copied original kernel config"
#     fi
    
#     # Check if fragments were applied and save the info
#     if [ -f "$LINUX_BUILD_DIR/.config" ]; then
#         echo "=== KERNEL PAGE SIZE CONFIG ===" > /workspace/output/logs/page-size-check.txt
#         grep -i page "$LINUX_BUILD_DIR/.config" >> /workspace/output/logs/page-size-check.txt || echo "No page size config found" >> /workspace/output/logs/page-size-check.txt
#         echo "" >> /workspace/output/logs/page-size-check.txt
#         echo "=== FRAGMENT APPLICATION CHECK ===" >> /workspace/output/logs/page-size-check.txt
#         grep -i fragment "$LINUX_BUILD_DIR"/* 2>/dev/null >> /workspace/output/logs/page-size-check.txt || echo "No fragment info found" >> /workspace/output/logs/page-size-check.txt
#     fi
# fi

# # Copy defconfig that was used
# if [ -f "$BUILD_DIR/.config" ]; then
#     cp "$BUILD_DIR/.config" /workspace/output/logs/buildroot.config
#     echo "Copied Buildroot config"
# fi

# # Create build summary
# echo "=== BUILD SUMMARY ===" > /workspace/output/logs/build-summary.txt
# echo "Build time: $(date)" >> /workspace/output/logs/build-summary.txt
# echo "Buildroot version: $(cat /opt/buildroot/Makefile | grep '^BR2_VERSION' | head -1)" >> /workspace/output/logs/build-summary.txt
# echo "External tree: /workspace/ZaraOS" >> /workspace/output/logs/build-summary.txt
# echo "" >> /workspace/output/logs/build-summary.txt

# # Document what was copied
# echo "=== COPIED ARTIFACTS ===" >> /workspace/output/logs/build-summary.txt
# echo "Complete build results copied to: /workspace/output/build-results/" >> /workspace/output/logs/build-summary.txt
# echo "Download cache copied to: /workspace/output/downloads/" >> /workspace/output/logs/build-summary.txt
# echo "Final images copied to: /workspace/output/" >> /workspace/output/logs/build-summary.txt
# if [ -d "/workspace/output/build-results" ]; then
#     echo "Build results directory size: $(du -sh /workspace/output/build-results | cut -f1)" >> /workspace/output/logs/build-summary.txt
# fi
# if [ -d "/workspace/output/downloads" ]; then
#     echo "Downloads directory size: $(du -sh /workspace/output/downloads | cut -f1)" >> /workspace/output/logs/build-summary.txt
# fi
# echo "" >> /workspace/output/logs/build-summary.txt

# Check kernel version in built image
if [ -f "/workspace/output/Image" ]; then
    echo "=== BUILT KERNEL INFO ===" >> /workspace/output/logs/build-summary.txt
    strings "/workspace/output/Image" | grep "Linux version" | head -2 >> /workspace/output/logs/build-summary.txt
    file "/workspace/output/Image" >> /workspace/output/logs/build-summary.txt
fi

echo "Build complete!"
echo "Final images available in: /workspace/output/"

