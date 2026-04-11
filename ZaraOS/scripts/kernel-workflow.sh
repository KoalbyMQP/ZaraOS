#!/bin/bash
#
# ZaraOS Kernel Development Workflow
# Orchestrates build, test, and deploy steps
#
# Usage: ./kernel-workflow.sh [build|test|deploy|full]
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_step() { echo -e "\n${BLUE}>>> $1${NC}"; }
log_done() { echo -e "${GREEN}✓ $1${NC}"; }

show_menu() {
    cat << 'EOF'

╔════════════════════════════════════════════════════════════════╗
║          ZaraOS Kernel Development Workflow                   ║
╚════════════════════════════════════════════════════════════════╝

Choose workflow:
  1) build       - Compile kernel for Pi 5
  2) qemu        - Build for QEMU + boot in VM
  3) test        - Test existing build in QEMU
  4) deploy      - Deploy to Raspberry Pi 5
  5) full        - Build → Test → Deploy (Pi 5)
  6) show-logs   - View build logs
  7) help        - Show all options

EOF
}

workflow_build() {
    log_step "Building ZaraOS Kernel"
    "$SCRIPT_DIR/build-kernel-local.sh" -j "$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)"
    log_done "Kernel build complete"
}

workflow_qemu() {
    log_step "Building Kernel for QEMU"
    "$SCRIPT_DIR/build-kernel-local.sh" --qemu -j "$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)"
    log_done "QEMU kernel build complete"

    log_step "Booting Kernel in QEMU"
    "$SCRIPT_DIR/qemu-test-kernel.sh"
    log_done "QEMU testing complete"
}

workflow_test() {
    log_step "Testing Kernel in QEMU"

    if ! command -v qemu-system-aarch64 &> /dev/null; then
        echo -e "${RED}✗ QEMU not found${NC}"
        echo "Install with: brew install qemu"
        return 1
    fi

    "$SCRIPT_DIR/qemu-test-kernel.sh"
    log_done "QEMU testing complete"
}

workflow_deploy() {
    log_step "Deploying Kernel to Raspberry Pi 5"

    # Check if Pi is reachable
    if ! ping -c 1 zaraos &>/dev/null; then
        echo -e "${RED}✗ Cannot reach zaraos (Pi not on network)${NC}"
        echo "Make sure Pi is powered on and connected to network"
        return 1
    fi

    log_done "Pi is reachable"

    local archive="$PROJECT_ROOT/../kernel-build/output/zaraos-kernel-latest.tar.gz"

    # Find latest archive if not named explicitly
    if [ ! -f "$archive" ]; then
        archive=$(ls -t "$PROJECT_ROOT/../kernel-build/output"/zaraos-kernel-*.tar.gz 2>/dev/null | head -1)
        if [ -z "$archive" ]; then
            echo -e "${RED}✗ No kernel archive found${NC}"
            echo "Build kernel first with: ./kernel-workflow.sh build"
            return 1
        fi
    fi

    log_step "Transferring kernel to Pi..."
    scp "$archive" pi@zaraos:/tmp/

    log_step "Installing on Pi..."
    ssh pi@zaraos << 'INSTALL_SCRIPT'
set -e
cd /tmp
archive=$(ls -t zaraos-kernel-*.tar.gz 2>/dev/null | head -1)

echo "Extracting: $archive"
tar xzf "$archive"

echo "Backing up current kernel..."
sudo cp /boot/Image /boot/Image.backup

echo "Installing new kernel..."
sudo cp Image /boot/
[ -d dtb ] && sudo cp dtb/* /boot/overlays/ 2>/dev/null || true

echo "Installing modules..."
if [ -d modules ]; then
    sudo cp -r modules/lib/modules/* /lib/modules/
    sudo depmod -a
fi

echo "Kernel installed successfully!"
echo ""
echo "To boot the new kernel, run: sudo reboot"
echo "To revert: sudo cp /boot/Image.backup /boot/Image && sudo reboot"
INSTALL_SCRIPT

    log_done "Kernel deployed to Pi"
    echo -e "\n${BLUE}Next steps:${NC}"
    echo "1. ssh pi@zaraos"
    echo "2. sudo reboot"
    echo "3. Verify: uname -a"
}

workflow_show_logs() {
    local log_file="$PROJECT_ROOT/../kernel-build/output/build.log"

    if [ ! -f "$log_file" ]; then
        echo "No build log found. Build kernel first with: ./kernel-workflow.sh build"
        return 1
    fi

    less "$log_file"
}

show_help() {
    cat << 'EOF'
ZaraOS Kernel Workflow Script

Usage: ./kernel-workflow.sh [command] [options]

Commands:
  build          Build kernel for Raspberry Pi 5
  qemu           Build for QEMU + boot in VM (recommended for testing)
  test           Test existing kernel build in QEMU VM
  deploy         Deploy kernel to Pi 5 over SSH
  full           Complete workflow (build → test → deploy) for Pi 5
  show-logs      View build logs
  status         Show build status and output locations
  help           Show this help message

Examples:
  # Quick QEMU test cycle (build + boot in VM)
  ./kernel-workflow.sh qemu

  # Build for Pi 5 with menuconfig
  ./kernel-workflow.sh build -- --menuconfig

  # Full Pi 5 workflow with custom parallelism
  ./kernel-workflow.sh full -- -j 8

  # Test existing build in QEMU with 8GB RAM
  ./kernel-workflow.sh test -- --memory 8

Options passed with '--' are forwarded to build or test scripts.

Environment:
  PARALLEL_JOBS  Set number of build jobs
  WORK_DIR       Set build directory location

EOF
}

show_status() {
    local output_dir="$PROJECT_ROOT/../kernel-build/output"

    echo -e "\n${BLUE}ZaraOS Kernel Build Status${NC}"
    echo "=============================="

    if [ -f "$output_dir/Image" ]; then
        local size=$(ls -lh "$output_dir/Image" | awk '{print $5}')
        echo -e "✓ Kernel Image: $size"
        echo "  Location: $output_dir/Image"
    else
        echo -e "${RED}✗ Kernel Image not found${NC}"
    fi

    if [ -d "$output_dir/modules" ]; then
        local mod_count=$(find "$output_dir/modules" -name "*.ko" 2>/dev/null | wc -l)
        echo -e "✓ Modules: $mod_count .ko files"
        echo "  Location: $output_dir/modules"
    else
        echo -e "- Modules directory not found"
    fi

    if [ -d "$output_dir/dtb" ]; then
        local dtb_count=$(find "$output_dir/dtb" -name "*.dtb" 2>/dev/null | wc -l)
        echo -e "✓ Device Trees: $dtb_count .dtb files"
        echo "  Location: $output_dir/dtb"
    else
        echo -e "- Device trees not found"
    fi

    if [ -f "$output_dir/build.log" ]; then
        echo -e "✓ Build log available"
        echo "  View with: ./kernel-workflow.sh show-logs"
    fi

    echo ""
}

# ============================================================================
# Main
# ============================================================================

main() {
    cd "$SCRIPT_DIR"

    local command="${1:-}"
    shift || true

    case "$command" in
        build)
            workflow_build "$@"
            ;;
        qemu)
            workflow_qemu "$@"
            ;;
        test)
            workflow_test "$@"
            ;;
        deploy)
            workflow_deploy "$@"
            ;;
        full)
            workflow_build "$@" && \
            workflow_test "$@" && \
            echo "" && \
            read -p "Deploy to Pi? (y/n) " -n 1 -r && echo && \
            [[ $REPLY =~ ^[Yy]$ ]] && workflow_deploy "$@"
            ;;
        show-logs)
            workflow_show_logs
            ;;
        status)
            show_status
            ;;
        help|--help|-h)
            show_help
            ;;
        "")
            show_menu
            read -p "Choose option (1-7): " choice
            case "$choice" in
                1) workflow_build ;;
                2) workflow_qemu ;;
                3) workflow_test ;;
                4) workflow_deploy ;;
                5) workflow_build && workflow_test ;;
                6) workflow_show_logs ;;
                *) show_help ;;
            esac
            ;;
        *)
            echo "Unknown command: $command"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
