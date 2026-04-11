#!/bin/bash
#
# ZaraOS Local Kernel Build Script
# Compiles the Raspberry Pi 5 kernel locally without needing full Buildroot
# Supports testing in QEMU VM or deploying to physical Pi
#
# Usage: ./build-kernel-local.sh [options]
#   --help          Show this help message
#   --clean         Clean previous builds
#   --menuconfig    Launch kernel menuconfig before building
#   --qemu          Build for QEMU testing (4K pages, virtio, initramfs)
#   --pi5           Build for Raspberry Pi 5 (default)
#   -j N            Number of parallel jobs (default: auto-detect)
#

set -eo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
WORK_DIR="${WORK_DIR:-$PROJECT_ROOT/../kernel-build}"
DOWNLOAD_DIR="$WORK_DIR/downloads"
BUILD_DIR="$WORK_DIR/build"
OUTPUT_DIR="$WORK_DIR/output"

# Kernel configuration
KERNEL_REPO="https://github.com/raspberrypi/linux"
KERNEL_VERSION="stable_20250916"  # Must match zaraos_pi5_defconfig
KERNEL_DEFCONFIG="bcm2712"
KERNEL_FRAGMENT="linux-pi5.fragment"
ARCH="arm64"
CROSS_COMPILE="${CROSS_COMPILE:-aarch64-linux-gnu-}"

# Host compiler for kernel build scripts (fixdep, sorttable, etc.)
# The kernel Makefile defaults to HOSTCC=gcc, but on macOS /usr/bin/gcc is an
# Apple shim that fails unless full Xcode CLT is installed. Prefer 'cc' which
# reliably points to the real compiler (clang in Nix, or Xcode clang).
if [ -n "${HOSTCC:-}" ]; then
    :  # respect user/env override
elif command -v cc &>/dev/null; then
    HOSTCC="cc"
else
    HOSTCC="gcc"
fi

# Host compiler flags — on macOS we need elf.h from the cross-toolchain's
# glibc headers since macOS uses Mach-O and has no native elf.h.
# The nix flake sets HOSTCFLAGS via shellHook; this is a fallback.
HOSTCFLAGS="${HOSTCFLAGS:-}"

# BusyBox for QEMU initramfs (statically-linked aarch64 binary)
BUSYBOX_VERSION="1.36.1"
BUSYBOX_URL="https://busybox.net/downloads/binaries/${BUSYBOX_VERSION}-defconfig-multiarch-musl/busybox-aarch64"

# Build options
PARALLEL_JOBS=${PARALLEL_JOBS:-$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)}
ENABLE_MENUCONFIG=0
DO_CLEAN=0
TARGET_PLATFORM="pi5"  # pi5 or qemu

# Helper: invoke make with all the cross-compile and host-compiler settings.
# Usage: kmake [extra make args...]
kmake() {
    local hostcflags_args=()
    if [ -n "$HOSTCFLAGS" ]; then
        hostcflags_args=(HOSTCFLAGS="$HOSTCFLAGS")
    fi
    make ARCH="$ARCH" CROSS_COMPILE="$CROSS_COMPILE" HOSTCC="$HOSTCC" "${hostcflags_args[@]}" "$@"
}

# ============================================================================
# Colors for output
# ============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# Helper Functions
# ============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

check_dependencies() {
    log_info "Checking dependencies..."
    log_info "Host compiler (HOSTCC): $HOSTCC"

    local missing=()

    # Essential build tools
    for cmd in git make bc bison flex; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done

    # Host C compiler (cc or gcc — we already resolved HOSTCC above)
    if ! command -v "$HOSTCC" &> /dev/null; then
        missing+=("$HOSTCC")
    fi

    # Cross-compiler
    if ! command -v "${CROSS_COMPILE}gcc" &> /dev/null; then
        missing+=("${CROSS_COMPILE}gcc")
    fi

    # Optional but recommended
    for cmd in ccache rsync; do
        if ! command -v "$cmd" &> /dev/null; then
            log_warn "Optional: $cmd not found (speeds up builds)"
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error ""
        log_error "Recommended: use the Nix dev shell which provides everything:"
        log_error "  nix develop"
        log_error ""
        log_error "Or install manually:"
        log_error "  macOS:  brew install make gcc@14 bison flex"
        log_error "  Linux:  sudo apt install build-essential bison flex libssl-dev gcc-aarch64-linux-gnu"
        return 1
    fi

    log_success "All dependencies satisfied"
    return 0
}

setup_directories() {
    log_info "Setting up build directories..."
    mkdir -p "$DOWNLOAD_DIR" "$BUILD_DIR" "$OUTPUT_DIR"
    log_success "Directories ready: $WORK_DIR"
}

download_kernel() {
    log_info "Downloading kernel source (${KERNEL_VERSION})..."

    local tarball="$DOWNLOAD_DIR/linux-${KERNEL_VERSION}.tar.gz"
    local kernel_src="$DOWNLOAD_DIR/linux-${KERNEL_VERSION}"

    if [ -d "$kernel_src" ]; then
        log_info "Kernel source already downloaded, skipping download"
        echo "$kernel_src"
        return 0
    fi

    if [ -f "$tarball" ]; then
        log_info "Tarball exists, extracting..."
    else
        log_info "Fetching kernel from $KERNEL_REPO..."
        # Redirect all subprocess output to stderr so stdout stays clean for path capture
        git clone --depth 1 --branch "$KERNEL_VERSION" "$KERNEL_REPO" "$kernel_src" >&2 2>&1 || {
            # Fallback to archive download if git clone fails
            log_info "Git clone failed, trying tarball download..."
            curl -L -o "$tarball" \
                "$KERNEL_REPO/archive/$KERNEL_VERSION.tar.gz" \
                --progress-bar >&2
            tar xzf "$tarball" -C "$DOWNLOAD_DIR" >&2
        }
    fi

    if [ -f "$tarball" ] && [ ! -d "$kernel_src" ]; then
        tar xzf "$tarball" -C "$DOWNLOAD_DIR" >&2
    fi

    if [ ! -d "$kernel_src" ]; then
        log_error "Failed to obtain kernel source"
        return 1
    fi

    log_success "Kernel source ready: $kernel_src"
    echo "$kernel_src"
}

configure_kernel() {
    local kernel_src="$1"

    log_info "Configuring kernel..."

    if [ ! -d "$kernel_src" ]; then
        log_error "Kernel source directory not found: $kernel_src"
        return 1
    fi
    cd "$kernel_src" || { log_error "Failed to cd into $kernel_src"; return 1; }

    # Select defconfig and fragment based on target platform
    local defconfig
    local fragment

    if [ "$TARGET_PLATFORM" = "qemu" ]; then
        defconfig="defconfig"  # Generic arm64 defconfig for QEMU virt
        fragment="$PROJECT_ROOT/ZaraOS/kernel-fragments/linux-qemu.fragment"
        log_info "Using generic arm64 defconfig (QEMU virt)..."
    else
        defconfig="bcm2712_defconfig"  # Pi 5 specific
        fragment="$PROJECT_ROOT/ZaraOS/kernel-fragments/linux-pi5.fragment"
        log_info "Using bcm2712 defconfig (Pi 5)..."
    fi

    kmake "$defconfig"

    # Apply ZaraOS kernel fragment using the kernel's own merge tool.
    # This properly handles CONFIG_FOO=n to disable options (unlike raw cat).
    # We pass HOSTCC/HOSTCFLAGS so merge_config.sh's internal make calls work.
    if [ -f "$fragment" ]; then
        log_info "Merging kernel fragment: $(basename "$fragment")..."
        ARCH="$ARCH" CROSS_COMPILE="$CROSS_COMPILE" \
            HOSTCC="$HOSTCC" HOSTCFLAGS="$HOSTCFLAGS" \
            ./scripts/kconfig/merge_config.sh -m .config "$fragment"
    else
        log_warn "Kernel fragment not found: $fragment"
        kmake olddefconfig
    fi

    # Launch menuconfig if requested
    if [ $ENABLE_MENUCONFIG -eq 1 ]; then
        log_info "Launching menuconfig for manual configuration..."
        kmake menuconfig
        # Save configuration for future reference
        cp .config "$OUTPUT_DIR/kernel-config-$(date +%s)"
    fi

    log_success "Kernel configured"
}

build_kernel() {
    local kernel_src="$1"

    log_info "Building kernel (${PARALLEL_JOBS} jobs)..."

    cd "$kernel_src" || { log_error "Failed to cd into $kernel_src"; return 1; }

    # Use ccache if available to speed up rebuilds
    local cc_prefix=""
    if command -v ccache &> /dev/null; then
        cc_prefix="ccache "
        log_info "Using ccache for faster compilation"
    fi

    # Build the kernel
    kmake \
        -j "$PARALLEL_JOBS" \
        CROSS_COMPILE="${cc_prefix}${CROSS_COMPILE}" \
        CC="${cc_prefix}${CROSS_COMPILE}gcc" \
        2>&1 | tee "$OUTPUT_DIR/build.log"

    log_success "Kernel build complete"
}

install_kernel() {
    local kernel_src="$1"

    log_info "Installing kernel to output directory..."

    cd "$kernel_src" || { log_error "Failed to cd into $kernel_src"; return 1; }

    # Copy kernel image
    if [ -f "arch/$ARCH/boot/Image" ]; then
        cp "arch/$ARCH/boot/Image" "$OUTPUT_DIR/Image"
        log_info "Kernel image: $OUTPUT_DIR/Image"
    else
        log_error "Kernel image not found at arch/$ARCH/boot/Image"
        return 1
    fi

    if [ "$TARGET_PLATFORM" = "qemu" ]; then
        # QEMU virt machine generates its own DTB; skip Pi 5 DTBs and modules
        # (drivers are built-in via the QEMU fragment, not loadable modules)
        log_info "QEMU build: skipping DTB copy and module install (built-in)"
    else
        # Copy Pi 5 device trees
        if [ -d "arch/$ARCH/boot/dts" ]; then
            mkdir -p "$OUTPUT_DIR/dtb"
            cp arch/$ARCH/boot/dts/broadcom/*.dtb "$OUTPUT_DIR/dtb/" 2>/dev/null || true
            log_info "Device trees: $OUTPUT_DIR/dtb/"
        fi

        # Install modules
        log_info "Installing kernel modules..."
        kmake \
            -j "$PARALLEL_JOBS" \
            INSTALL_MOD_PATH="$OUTPUT_DIR/modules" \
            modules_install 2>&1 | tee -a "$OUTPUT_DIR/build.log"
    fi

    log_success "Kernel installation complete"
}

get_busybox() {
    # Prefer BUSYBOX_BIN from environment (set by nix flake)
    if [ -n "${BUSYBOX_BIN:-}" ] && [ -f "$BUSYBOX_BIN" ]; then
        log_info "Using BusyBox from nix: $BUSYBOX_BIN"
        echo "$BUSYBOX_BIN"
        return 0
    fi

    # Fallback: download a static binary
    local busybox_bin="$DOWNLOAD_DIR/busybox-aarch64"

    if [ -f "$busybox_bin" ]; then
        # Verify it's actually a binary, not an HTML 404 page
        if file "$busybox_bin" | grep -q "ELF"; then
            log_info "BusyBox already downloaded"
            echo "$busybox_bin"
            return 0
        else
            log_warn "Cached BusyBox is invalid, re-downloading..."
            rm -f "$busybox_bin"
        fi
    fi

    log_info "Downloading statically-linked BusyBox (aarch64)..."
    curl -L -f -o "$busybox_bin" "$BUSYBOX_URL" --progress-bar >&2 || {
        log_error "Failed to download BusyBox from $BUSYBOX_URL"
        log_error "Use 'nix develop' which provides a pre-built BusyBox,"
        log_error "or manually place a static aarch64 busybox binary at: $busybox_bin"
        return 1
    }
    chmod +x "$busybox_bin"
    log_success "BusyBox downloaded: $busybox_bin"
    echo "$busybox_bin"
}

build_initramfs() {
    log_info "Building QEMU initramfs..."

    local busybox_bin
    busybox_bin=$(get_busybox) || return 1

    local initramfs_dir="$WORK_DIR/initramfs"

    # Clean previous initramfs
    rm -rf "$initramfs_dir"

    # Create directory structure
    mkdir -p "$initramfs_dir"/{bin,sbin,usr/bin,usr/sbin,proc,sys,dev,tmp,run,root,etc,mnt/host}

    # Copy BusyBox binary
    cp "$busybox_bin" "$initramfs_dir/bin/busybox"
    chmod +x "$initramfs_dir/bin/busybox"

    # Create symlinks for common BusyBox applets
    local applets=(
        sh ash bash
        ls cat echo mkdir rm cp mv ln
        mount umount
        ps kill sleep
        grep sed awk head tail wc sort uniq tr cut
        dmesg sysctl hostname uname
        ifconfig ip route ping
        vi less more
        tar gzip gunzip
        chmod chown chgrp id whoami
        df du free top uptime
        mknod mktemp
        find xargs
        clear reset
        reboot poweroff halt
        init
    )

    for applet in "${applets[@]}"; do
        ln -sf busybox "$initramfs_dir/bin/$applet"
    done

    # Also create sbin symlinks
    for applet in init reboot poweroff halt; do
        ln -sf ../bin/busybox "$initramfs_dir/sbin/$applet"
    done

    # Create /init script
    cat > "$initramfs_dir/init" << 'INIT_EOF'
#!/bin/sh
# ZaraOS QEMU Test Environment - Init Script

# Mount essential filesystems
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mount -t tmpfs tmpfs /tmp
mount -t tmpfs tmpfs /run
mkdir -p /dev/pts && mount -t devpts devpts /dev/pts

# Set hostname
hostname zaraos-qemu

# Configure PATH
export PATH=/bin:/sbin:/usr/bin:/usr/sbin
export HOME=/root
export TERM=linux

# Print banner
echo ""
echo "  ╔══════════════════════════════════════════════════╗"
echo "  ║        ZaraOS QEMU Test Environment              ║"
echo "  ╚══════════════════════════════════════════════════╝"
echo ""
echo "  Kernel:  $(uname -r)"
echo "  Arch:    $(uname -m)"
echo "  Machine: $(uname -n)"
echo ""

# Show memory info
echo "  Memory:"
free -h 2>/dev/null | head -2 | sed 's/^/    /'
echo ""

# Try to mount 9p host share if available
if grep -q 9p /proc/filesystems 2>/dev/null; then
    mount -t 9p -o trans=virtio host0 /mnt/host 2>/dev/null && \
        echo "  Host share mounted at /mnt/host" && echo ""
fi

echo "  Commands: dmesg, uname -a, cat /proc/cpuinfo"
echo "  Exit:     poweroff  (or Ctrl+A then X)"
echo ""

# Drop to interactive shell
exec /bin/sh
INIT_EOF

    chmod +x "$initramfs_dir/init"

    # Create /etc files
    echo "zaraos-qemu" > "$initramfs_dir/etc/hostname"
    echo "root:x:0:0:root:/root:/bin/sh" > "$initramfs_dir/etc/passwd"
    echo "root:x:0:" > "$initramfs_dir/etc/group"

    # Create the cpio initramfs archive
    log_info "Packing initramfs..."
    (cd "$initramfs_dir" && find . | cpio -o -H newc 2>/dev/null | gzip > "$OUTPUT_DIR/initramfs.cpio.gz")

    local size
    size=$(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')
    log_success "Initramfs created: $OUTPUT_DIR/initramfs.cpio.gz ($size)"
}

create_deployment_archive() {
    log_info "Creating deployment archive..."

    local archive="$OUTPUT_DIR/zaraos-kernel-$(date +%Y%m%d-%H%M%S).tar.gz"

    cd "$OUTPUT_DIR"
    tar czf "$archive" \
        Image \
        dtb/ \
        modules/ \
        2>/dev/null || true

    log_success "Deployment archive: $archive"
}

show_qemu_instructions() {
    cat >&2 << EOF

╔════════════════════════════════════════════════════════════════╗
║  Next Steps: Test Kernel in QEMU VM                           ║
╚════════════════════════════════════════════════════════════════╝

1. Install QEMU (if not already installed):
   macOS:  brew install qemu
   Linux:  sudo apt install qemu-system-arm

2. Run the kernel in QEMU:
   $SCRIPT_DIR/qemu-test-kernel.sh

   This will auto-detect:
     Kernel:    $OUTPUT_DIR/Image
     Initramfs: $OUTPUT_DIR/initramfs.cpio.gz

3. Or use the workflow script:
   $SCRIPT_DIR/kernel-workflow.sh qemu

Keyboard shortcuts in QEMU:
  Ctrl+A X  — Exit QEMU
  Ctrl+A C  — QEMU monitor console
  Ctrl+A H  — Help

EOF
}

show_pi_deployment_instructions() {
    cat >&2 << EOF

╔════════════════════════════════════════════════════════════════╗
║  Next Steps: Deploy to Raspberry Pi 5                         ║
╚════════════════════════════════════════════════════════════════╝

1. Extract deployment archive on a Pi 5 running ZaraOS:
   ssh pi@zaraos
   cd /tmp
   tar xzf zaraos-kernel-*.tar.gz

2. Backup current kernel:
   sudo cp /boot/Image /boot/Image.backup

3. Install new kernel:
   sudo cp Image /boot/
   sudo cp dtb/* /boot/overlays/  # or appropriate location

4. Install modules:
   sudo cp -r modules/lib/modules/* /lib/modules/

5. Reboot and test:
   sudo reboot

6. Verify kernel version:
   uname -a

Monitor kernel logs during boot:
   dmesg | tail -50

EOF
}

show_help() {
    cat << EOF
ZaraOS Local Kernel Build Script

Usage: $0 [options]

Options:
  --help            Show this help message
  --clean           Clean all previous build artifacts
  --menuconfig      Launch kernel menuconfig before building
  --qemu            Configure for QEMU testing
  --pi5             Configure for Raspberry Pi 5 (default)
  -j N              Number of parallel build jobs (default: $PARALLEL_JOBS)

Examples:
  # Standard build for Pi 5
  ./build-kernel-local.sh

  # Build with custom parallelism and menuconfig
  ./build-kernel-local.sh -j 8 --menuconfig

  # Clean build
  ./build-kernel-local.sh --clean

Environment Variables:
  WORK_DIR         Build directory (default: ../kernel-build)
  PARALLEL_JOBS    Parallel build jobs (default: auto-detect)
  CROSS_COMPILE    Cross-compiler prefix (default: aarch64-linux-gnu-)

Output:
  Kernel image:    $OUTPUT_DIR/Image
  Modules:         $OUTPUT_DIR/modules/
  Device trees:    $OUTPUT_DIR/dtb/
  Archive:         $OUTPUT_DIR/zaraos-kernel-*.tar.gz

EOF
}

# ============================================================================
# Main Script
# ============================================================================

main() {
    # Parse arguments first so banner shows correct platform
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help) show_help; exit 0 ;;
            --clean) DO_CLEAN=1 ;;
            --menuconfig) ENABLE_MENUCONFIG=1 ;;
            --qemu) TARGET_PLATFORM="qemu" ;;
            --pi5) TARGET_PLATFORM="pi5" ;;
            -j) PARALLEL_JOBS="$2"; shift ;;
            *) log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
        shift
    done

    log_info "ZaraOS Kernel Build Script"
    log_info "Architecture: $ARCH | Platform: $TARGET_PLATFORM | Jobs: $PARALLEL_JOBS"
    echo "" >&2

    # Cleanup if requested
    if [ $DO_CLEAN -eq 1 ]; then
        log_warn "Cleaning build directory..."
        rm -rf "$BUILD_DIR" "$DOWNLOAD_DIR"
        log_success "Cleaned"
        exit 0
    fi

    # Execute build pipeline
    # Note: call each step directly (not with || exit 1) so that set -e
    # propagates into the functions and catches individual command failures.
    check_dependencies
    setup_directories

    KERNEL_SRC=$(download_kernel)
    configure_kernel "$KERNEL_SRC"
    build_kernel "$KERNEL_SRC"
    install_kernel "$KERNEL_SRC"

    # Post-build: create initramfs for QEMU or deployment archive for Pi 5
    echo "" >&2
    if [ "$TARGET_PLATFORM" = "qemu" ]; then
        build_initramfs
        show_qemu_instructions
    else
        create_deployment_archive
        show_pi_deployment_instructions
    fi

    log_success "Build complete! Output in: $OUTPUT_DIR"
}

main "$@"
