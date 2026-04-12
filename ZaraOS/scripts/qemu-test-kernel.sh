#!/bin/bash
#
# ZaraOS QEMU Kernel Testing Script
# Boots the locally-compiled kernel in a QEMU aarch64 VM for testing
#
# Usage: ./qemu-test-kernel.sh [options]
#   --kernel IMAGE      Path to kernel Image file
#   --initramfs FILE    Path to initramfs cpio archive
#   --rootfs IMG        Path to rootfs ext4 image (alternative to initramfs)
#   --shared-dir DIR    Share a host directory via 9P (mounted at /mnt/host)
#   --memory N          RAM in GB (default: 4)
#   --cpus N            Number of vCPUs (default: 4)
#   --gdb               Enable GDB server on port 1234 (pauses at boot)
#   --help              Show this help message
#
# Prerequisites:
#   1. Build the kernel: ./build-kernel-local.sh --qemu
#   2. Install QEMU:    brew install qemu  (macOS)
#                        sudo apt install qemu-system-arm  (Linux)
#

set -e

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
WORK_DIR="${WORK_DIR:-$PROJECT_ROOT/../kernel-build}"
OUTPUT_DIR="$WORK_DIR/output"

# QEMU configuration
QEMU_MEMORY="4G"
QEMU_CPUS="4"
MACHINE_TYPE="virt"

# Paths (auto-detected from build output)
KERNEL_IMAGE=""
INITRAMFS=""
ROOTFS_IMAGE=""
SHARED_DIR=""
ENABLE_GDB=0

# ============================================================================
# Colors
# ============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ============================================================================
# Helper Functions
# ============================================================================

check_qemu() {
    if ! command -v qemu-system-aarch64 &> /dev/null; then
        log_error "QEMU not found. Install with:"
        echo "  macOS:  brew install qemu"
        echo "  Linux:  sudo apt install qemu-system-arm"
        return 1
    fi

    local qemu_version
    qemu_version=$(qemu-system-aarch64 --version | head -1)
    log_success "QEMU: $qemu_version"
}

detect_cpu_type() {
    # Check if QEMU supports cortex-a76; fall back to "max" if not
    if qemu-system-aarch64 -cpu help 2>/dev/null | grep -q "cortex-a76"; then
        echo "cortex-a76"
    else
        log_warn "cortex-a76 not available in this QEMU version, using 'max'"
        echo "max"
    fi
}

auto_detect_files() {
    # Auto-detect kernel image
    if [ -z "$KERNEL_IMAGE" ]; then
        if [ -f "$OUTPUT_DIR/Image" ]; then
            KERNEL_IMAGE="$OUTPUT_DIR/Image"
        fi
    fi

    if [ -z "$KERNEL_IMAGE" ] || [ ! -f "$KERNEL_IMAGE" ]; then
        log_error "Kernel image not found."
        log_info "Build kernel first with: $SCRIPT_DIR/build-kernel-local.sh --qemu"
        [ -n "$KERNEL_IMAGE" ] && log_info "Looked for: $KERNEL_IMAGE"
        return 1
    fi

    local kernel_size
    kernel_size=$(ls -lh "$KERNEL_IMAGE" | awk '{print $5}')
    log_success "Kernel: $KERNEL_IMAGE ($kernel_size)"

    # Auto-detect initramfs
    if [ -z "$INITRAMFS" ] && [ -z "$ROOTFS_IMAGE" ]; then
        if [ -f "$OUTPUT_DIR/initramfs.cpio.gz" ]; then
            INITRAMFS="$OUTPUT_DIR/initramfs.cpio.gz"
        fi
    fi

    if [ -n "$INITRAMFS" ] && [ -f "$INITRAMFS" ]; then
        local initramfs_size
        initramfs_size=$(ls -lh "$INITRAMFS" | awk '{print $5}')
        log_success "Initramfs: $INITRAMFS ($initramfs_size)"
    elif [ -n "$ROOTFS_IMAGE" ] && [ -f "$ROOTFS_IMAGE" ]; then
        local rootfs_size
        rootfs_size=$(ls -lh "$ROOTFS_IMAGE" | awk '{print $5}')
        log_success "Rootfs: $ROOTFS_IMAGE ($rootfs_size)"
    else
        log_error "No initramfs or rootfs found."
        log_info "Build with QEMU support: $SCRIPT_DIR/build-kernel-local.sh --qemu"
        log_info "This creates an initramfs automatically."
        return 1
    fi

    if [ -n "$SHARED_DIR" ]; then
        if [ -d "$SHARED_DIR" ]; then
            log_success "Shared dir: $SHARED_DIR -> /mnt/host in VM"
        else
            log_error "Shared directory not found: $SHARED_DIR"
            return 1
        fi
    fi
}

run_qemu() {
    local cpu_type
    cpu_type=$(detect_cpu_type)

    log_info "Starting QEMU..."
    log_info "  Machine: $MACHINE_TYPE"
    log_info "  CPU:     $cpu_type x $QEMU_CPUS"
    log_info "  Memory:  $QEMU_MEMORY"
    echo ""

    # Build QEMU command
    local qemu_args=(
        "-M" "$MACHINE_TYPE"
        "-cpu" "$cpu_type"
        "-m" "$QEMU_MEMORY"
        "-smp" "$QEMU_CPUS"
        "-kernel" "$KERNEL_IMAGE"
        "-nographic"
        "-no-reboot"
    )

    # Determine boot mode: initramfs vs rootfs disk
    if [ -n "$INITRAMFS" ] && [ -f "$INITRAMFS" ]; then
        # Boot with initramfs (self-contained, no disk needed)
        qemu_args+=(
            "-initrd" "$INITRAMFS"
            "-append" "console=ttyAMA0 rdinit=/init loglevel=4"
        )
    elif [ -n "$ROOTFS_IMAGE" ] && [ -f "$ROOTFS_IMAGE" ]; then
        # Boot with rootfs disk image
        qemu_args+=(
            "-drive" "file=$ROOTFS_IMAGE,format=raw,if=virtio"
            "-append" "root=/dev/vda rw console=ttyAMA0 loglevel=4"
        )
    fi

    # Add 9P host filesystem sharing
    if [ -n "$SHARED_DIR" ] && [ -d "$SHARED_DIR" ]; then
        qemu_args+=(
            "-virtfs" "local,path=$SHARED_DIR,mount_tag=host0,security_model=mapped-xattr,id=host0"
        )
    fi

    # Add GDB server if requested
    if [ $ENABLE_GDB -eq 1 ]; then
        qemu_args+=("-s" "-S")
        log_info "GDB server listening on localhost:1234 (VM paused, waiting for debugger)"
        log_info "Connect with: gdb-multiarch -ex 'target remote :1234' vmlinux"
    fi

    echo "────────────────────────────────────────────────────────────────"
    log_info "QEMU command:"
    echo "  qemu-system-aarch64 \\"
    local i=0
    while [ $i -lt ${#qemu_args[@]} ]; do
        if [ $((i + 1)) -lt ${#qemu_args[@]} ] && [[ "${qemu_args[$((i+1))]}" != -* ]]; then
            echo "    ${qemu_args[$i]} ${qemu_args[$((i+1))]} \\"
            i=$((i + 2))
        else
            echo "    ${qemu_args[$i]} \\"
            i=$((i + 1))
        fi
    done
    echo ""
    echo "  Ctrl+A X  — Exit QEMU"
    echo "  Ctrl+A C  — QEMU monitor"
    echo "────────────────────────────────────────────────────────────────"
    echo ""

    qemu-system-aarch64 "${qemu_args[@]}"
}

show_help() {
    cat << EOF
ZaraOS QEMU Kernel Testing Script

Boots the locally-compiled ZaraOS kernel in a QEMU aarch64 VM.

Usage: $0 [options]

Options:
  --kernel IMAGE      Path to kernel Image (default: auto-detect from build output)
  --initramfs FILE    Path to initramfs cpio.gz (default: auto-detect)
  --rootfs IMG        Path to rootfs ext4 image (alternative to initramfs)
  --shared-dir DIR    Share a host directory via 9P (mounted at /mnt/host in VM)
  --memory N          RAM in GB (default: 4)
  --cpus N            Number of vCPUs (default: 4)
  --gdb               Enable GDB server on localhost:1234 (pauses at boot)
  --help              Show this help message

Quick Start:
  # Build kernel for QEMU (creates kernel + initramfs automatically)
  $SCRIPT_DIR/build-kernel-local.sh --qemu

  # Boot in QEMU (auto-detects kernel and initramfs)
  $0

  # Or share a host directory into the VM
  $0 --shared-dir /path/to/project

  # In the VM, access it at /mnt/host

Keyboard Shortcuts:
  Ctrl+A X        Exit QEMU
  Ctrl+A C        Toggle QEMU monitor / serial console
  Ctrl+A H        Help

EOF
}

# ============================================================================
# Main
# ============================================================================

main() {
    log_info "ZaraOS QEMU Kernel Tester"
    echo ""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help) show_help; exit 0 ;;
            --kernel) KERNEL_IMAGE="$2"; shift ;;
            --initramfs) INITRAMFS="$2"; shift ;;
            --rootfs) ROOTFS_IMAGE="$2"; shift ;;
            --shared-dir) SHARED_DIR="$2"; shift ;;
            --memory) QEMU_MEMORY="${2}G"; shift ;;
            --cpus) QEMU_CPUS="$2"; shift ;;
            --gdb) ENABLE_GDB=1 ;;
            *) log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
        shift
    done

    # Validate setup
    check_qemu || exit 1
    auto_detect_files || exit 1

    # Run QEMU
    run_qemu
}

main "$@"
