#!/bin/bash
#
# ZaraOS Buildroot QEMU Build Script
# ===================================
# Builds the FULL ZaraOS distribution targeting QEMU, using Buildroot.
# Produces an identical rootfs to the Pi 5 build (same packages, same
# toolchain, same glibc) — just with different hardware drivers.
#
# Usage:
#   ./build-qemu-buildroot.sh              # Full build (first time: ~30-60 min)
#   ./build-qemu-buildroot.sh --rebuild    # Incremental rebuild (~2-5 min)
#   ./build-qemu-buildroot.sh --menuconfig # Open kernel menuconfig
#   ./build-qemu-buildroot.sh --clean      # Wipe build directory
#
# Prerequisites:
#   Linux:  sudo apt install build-essential gcc g++ bison flex libssl-dev ...
#   macOS:  Uses Docker automatically (Linux required for Buildroot)
#

set -eo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
ZARAOS_DIR="$PROJECT_ROOT/ZaraOS"
BUILDROOT_DIR="$ZARAOS_DIR/buildroot"

# Build directories
BUILD_DIR="${BUILD_DIR:-$PROJECT_ROOT/../buildroot-qemu-build}"
DL_DIR="${DL_DIR:-$PROJECT_ROOT/../buildroot-dl-cache}"
OUTPUT_DIR="$BUILD_DIR/images"

DEFCONFIG="zaraos_qemu_defconfig"
JOBS="${JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

# Docker settings (for macOS)
BUILDER_IMAGE="zaraos-builder:latest"

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[OK]${NC} $1" >&2; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_step()    { echo -e "\n${CYAN}━━━ $1 ━━━${NC}" >&2; }

# ============================================================================
# Buildroot submodule
# ============================================================================

init_submodule() {
    log_step "Initializing Buildroot submodule"

    if [ -f "$BUILDROOT_DIR/Makefile" ]; then
        log_info "Buildroot already initialized"
        return 0
    fi

    log_info "Cloning Buildroot (this is a one-time operation)..."
    cd "$PROJECT_ROOT"
    git submodule update --init --depth 1 ZaraOS/buildroot

    if [ ! -f "$BUILDROOT_DIR/Makefile" ]; then
        log_error "Failed to initialize Buildroot submodule"
        log_error "Try manually: git submodule update --init ZaraOS/buildroot"
        return 1
    fi

    log_success "Buildroot initialized: $(git -C "$BUILDROOT_DIR" describe --tags 2>/dev/null || echo 'unknown version')"
}

# ============================================================================
# Platform detection
# ============================================================================

detect_platform() {
    case "$(uname -s)" in
        Linux)  echo "linux" ;;
        Darwin) echo "macos" ;;
        *)      echo "unknown" ;;
    esac
}

check_linux_deps() {
    local missing=()
    for cmd in make gcc g++ bison flex bc cpio rsync unzip perl; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing build dependencies: ${missing[*]}"
        log_error "Install with: sudo apt install build-essential bison flex bc cpio rsync unzip perl libssl-dev libncurses-dev"
        return 1
    fi
    log_success "Build dependencies satisfied"
}

# ============================================================================
# Docker build (macOS)
# ============================================================================

build_docker_image() {
    log_step "Building Docker builder image"

    if docker image inspect "$BUILDER_IMAGE" &>/dev/null; then
        log_info "Builder image already exists"
        return 0
    fi

    log_info "Building zaraos-builder Docker image (one-time, ~5 min)..."
    cd "$PROJECT_ROOT"
    docker build -t "$BUILDER_IMAGE" -f infra/containers/builder/Dockerfile .
    log_success "Docker builder image ready"
}

run_in_docker() {
    local build_cmd="$1"

    log_info "Running Buildroot inside Docker..."

    mkdir -p "$BUILD_DIR" "$DL_DIR"

    docker run --rm \
        -v "$PROJECT_ROOT:/workspace" \
        -v "$BUILD_DIR:/tmp/zaraos-build" \
        -v "$DL_DIR:/tmp/zaraos-dl" \
        -e "WORKSPACE_PATH=/workspace" \
        -e "EXTERNAL_PATH=/workspace/ZaraOS" \
        -e "BUILD_DIR=/tmp/zaraos-build" \
        -e "DL_DIR=/tmp/zaraos-dl" \
        -e "OUTPUT_DIR=/workspace/output-qemu" \
        -e "DEFCONFIG=$DEFCONFIG" \
        -e "JOBS=$JOBS" \
        "$BUILDER_IMAGE" \
        bash -c "$build_cmd"
}

# ============================================================================
# Native Linux build
# ============================================================================

run_native_build() {
    local action="$1"

    mkdir -p "$BUILD_DIR" "$DL_DIR"

    case "$action" in
        configure)
            log_info "Configuring Buildroot with $DEFCONFIG..."
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                "$DEFCONFIG"
            log_success "Configured"
            ;;
        build)
            log_info "Building ZaraOS for QEMU ($JOBS parallel jobs)..."
            log_info "This will take 30-60 minutes on first build."
            log_info "Subsequent rebuilds are incremental (~2-5 min)."
            echo ""
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                -j"$JOBS"
            log_success "Build complete"
            ;;
        menuconfig)
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                menuconfig
            ;;
        linux-menuconfig)
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                linux-menuconfig
            ;;
        rebuild-cortex)
            log_info "Rebuilding Cortex package..."
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                cortex-rebuild
            # Regenerate rootfs with updated Cortex
            make -C "$BUILDROOT_DIR" \
                O="$BUILD_DIR" \
                BR2_DL_DIR="$DL_DIR" \
                BR2_EXTERNAL="$ZARAOS_DIR" \
                -j"$JOBS"
            log_success "Cortex rebuilt and rootfs regenerated"
            ;;
    esac
}

# ============================================================================
# Main
# ============================================================================

show_help() {
    cat << 'EOF'
ZaraOS Buildroot QEMU Build

Usage: ./build-qemu-buildroot.sh [command]

Commands:
  (default)          Full build (configure + build)
  --rebuild          Incremental rebuild (after code changes)
  --rebuild-cortex   Rebuild only Cortex, then regenerate rootfs
  --menuconfig       Open Buildroot menuconfig
  --linux-menuconfig Open kernel menuconfig
  --clean            Remove all build artifacts
  --help             Show this help

Build Times:
  First build:       30-60 minutes (downloads + compiles everything)
  Incremental:       2-5 minutes (only rebuilds changed packages)
  Cortex rebuild:    ~30 seconds

After build, run:
  <build-dir>/images/run-zaraos-qemu.sh         # Terminal mode
  <build-dir>/images/run-zaraos-qemu.sh --gui   # GUI mode

EOF
}

main() {
    local action="full"

    while [[ $# -gt 0 ]]; do
        case $1 in
            --rebuild)          action="rebuild" ;;
            --rebuild-cortex)   action="rebuild-cortex" ;;
            --menuconfig)       action="menuconfig" ;;
            --linux-menuconfig) action="linux-menuconfig" ;;
            --clean)
                log_warn "Removing build directory: $BUILD_DIR"
                rm -rf "$BUILD_DIR"
                log_success "Cleaned (download cache preserved at $DL_DIR)"
                exit 0
                ;;
            --help|-h) show_help; exit 0 ;;
            *) log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
        shift
    done

    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   ZaraOS Buildroot QEMU Build                               ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Step 1: Ensure Buildroot submodule is ready
    init_submodule

    # Step 2: Detect platform
    local platform
    platform=$(detect_platform)
    log_info "Platform: $platform | Jobs: $JOBS"

    if [ "$platform" = "macos" ]; then
        # macOS: must use Docker (Buildroot requires Linux)
        log_warn "macOS detected — Buildroot requires Linux."
        log_info "Using Docker to build..."

        if ! command -v docker &>/dev/null; then
            log_error "Docker not found. Install Docker Desktop for Mac."
            log_error "  https://docs.docker.com/desktop/install/mac-install/"
            exit 1
        fi

        build_docker_image

        case "$action" in
            full)
                run_in_docker "build-zaraos.sh"
                ;;
            rebuild)
                # For incremental rebuilds, skip the configure step
                run_in_docker "make -C /opt/buildroot O=/tmp/zaraos-build BR2_DL_DIR=/tmp/zaraos-dl BR2_EXTERNAL=/workspace/ZaraOS -j$JOBS"
                ;;
            *)
                log_error "Action '$action' requires native Linux (no Docker support)"
                log_error "Use a Linux VM or WSL for menuconfig."
                exit 1
                ;;
        esac
    else
        # Linux: build natively
        check_linux_deps

        case "$action" in
            full)
                run_native_build configure
                run_native_build build
                ;;
            rebuild)
                if [ ! -f "$BUILD_DIR/.config" ]; then
                    log_info "No existing build found, running full build..."
                    run_native_build configure
                fi
                run_native_build build
                ;;
            rebuild-cortex)
                run_native_build rebuild-cortex
                ;;
            menuconfig)
                if [ ! -f "$BUILD_DIR/.config" ]; then
                    run_native_build configure
                fi
                run_native_build menuconfig
                ;;
            linux-menuconfig)
                if [ ! -f "$BUILD_DIR/.config" ]; then
                    run_native_build configure
                fi
                run_native_build linux-menuconfig
                ;;
        esac
    fi

    # Step 3: Show results
    local images_dir="$BUILD_DIR/images"
    if [ -f "$images_dir/rootfs.ext4" ]; then
        echo ""
        log_success "Build complete!"
        echo ""
        echo -e "  ${GREEN}Artifacts:${NC}"
        echo "    Kernel:  $(du -h "$images_dir/Image" 2>/dev/null | cut -f1) — $images_dir/Image"
        echo "    Rootfs:  $(du -h "$images_dir/rootfs.ext4" 2>/dev/null | cut -f1) — $images_dir/rootfs.ext4"
        echo ""
        echo -e "  ${GREEN}Run:${NC}"
        echo "    $images_dir/run-zaraos-qemu.sh"
        echo "    $images_dir/run-zaraos-qemu.sh --gui"
        echo ""
        echo -e "  ${GREEN}Dev workflow (after code changes):${NC}"
        echo "    $0 --rebuild              # Rebuild everything changed"
        echo "    $0 --rebuild-cortex       # Rebuild Cortex only (~30s)"
        echo ""
    fi
}

main "$@"
