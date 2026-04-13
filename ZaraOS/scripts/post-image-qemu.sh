#!/bin/bash
set -e

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "ZaraOS Post-Image: QEMU Development Environment"
echo "═══════════════════════════════════════════════════════════════"

BOARD_DIR="${BR2_EXTERNAL_ZaraOS_PATH:-$(dirname "$(dirname "$0")")}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ VERSION                                                         │
# └─────────────────────────────────────────────────────────────────┘

VERSION_FILE="${BOARD_DIR}/VERSION"
if [ ! -f "$VERSION_FILE" ]; then
    echo "ERROR: VERSION file not found at ${VERSION_FILE}"
    exit 1
fi

# shellcheck source=/dev/null
. "$VERSION_FILE"
echo "Versions: BOARD=${BOARD_VERSION} OS=${OS_VERSION}"

# ┌─────────────────────────────────────────────────────────────────┐
# │ VALIDATE ARTIFACTS                                              │
# └─────────────────────────────────────────────────────────────────┘

for f in Image rootfs.ext4; do
    if [ ! -f "${BINARIES_DIR}/${f}" ]; then
        echo "ERROR: missing required file: ${f}"
        exit 1
    fi
    echo "  ${f}: $(du -h "${BINARIES_DIR}/${f}" | cut -f1)"
done

# ┌─────────────────────────────────────────────────────────────────┐
# │ CREATE QEMU DATA DISK                                          │
# └─────────────────────────────────────────────────────────────────┘

DATA_DISK="${BINARIES_DIR}/data.ext4"
if [ ! -f "$DATA_DISK" ]; then
    echo "Creating persistent data disk (1GB — room for container images)..."
    dd if=/dev/zero of="$DATA_DISK" bs=1M count=1024 status=none
    mkfs.ext4 -q -L zaraos-data "$DATA_DISK"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ GENERATE QEMU LAUNCH SCRIPT                                    │
# └─────────────────────────────────────────────────────────────────┘

cat > "${BINARIES_DIR}/run-zaraos-qemu.sh" << 'LAUNCHER'
#!/bin/bash
# ═══════════════════════════════════════════════════════════════
# ZaraOS QEMU Launcher — boots the Buildroot-built OS image
# ═══════════════════════════════════════════════════════════════
#
# Usage:
#   ./run-zaraos-qemu.sh              Terminal mode (serial console)
#   ./run-zaraos-qemu.sh --gui        Graphical window (like Pi HDMI)
#   ./run-zaraos-qemu.sh --shared-dir /path    Share host directory
#
# Access:
#   SSH:      ssh root@localhost -p 2222
#   Cortex:   curl http://localhost:8080/health
#   Monitor:  curl http://localhost:9100/metrics
#   Exit:     Ctrl+A X  (or 'poweroff' inside VM)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DISPLAY_MODE="nographic"
SHARED_DIR=""
ENABLE_GDB=0
EXTRA_ARGS=()
MEMORY="4G"
CPUS="4"

while [[ $# -gt 0 ]]; do
    case $1 in
        --gui)        DISPLAY_MODE="gui" ;;
        --shared-dir) SHARED_DIR="$2"; shift ;;
        --gdb)        ENABLE_GDB=1 ;;
        --memory)     MEMORY="${2}G"; shift ;;
        --cpus)       CPUS="$2"; shift ;;
        --help|-h)
            sed -n '2,/^$/s/^# \?//p' "$0"
            exit 0
            ;;
        *)            EXTRA_ARGS+=("$1") ;;
    esac
    shift
done

# Find QEMU
QEMU=""
for candidate in qemu-system-aarch64 /opt/homebrew/bin/qemu-system-aarch64 /usr/bin/qemu-system-aarch64; do
    if command -v "$candidate" &>/dev/null; then
        QEMU="$candidate"
        break
    fi
done
if [ -z "$QEMU" ]; then
    echo "ERROR: qemu-system-aarch64 not found"
    echo "  macOS:  brew install qemu"
    echo "  Linux:  sudo apt install qemu-system-arm"
    exit 1
fi

# Validate files
for f in Image rootfs.ext4; do
    if [ ! -f "$SCRIPT_DIR/$f" ]; then
        echo "ERROR: $f not found in $SCRIPT_DIR"
        exit 1
    fi
done

# Detect CPU
CPU="cortex-a76"
if ! $QEMU -cpu help 2>/dev/null | grep -q "$CPU"; then
    CPU="max"
fi

# Build QEMU command
QEMU_ARGS=(
    "-M" "virt"
    "-cpu" "$CPU"
    "-m" "$MEMORY"
    "-smp" "$CPUS"
    "-kernel" "$SCRIPT_DIR/Image"

    # Root filesystem (the REAL Buildroot rootfs — identical to Pi 5)
    "-drive" "file=$SCRIPT_DIR/rootfs.ext4,format=raw,if=virtio,id=rootfs"

    # Persistent data disk (mirrors /dev/mmcblk0p4 on Pi)
    "-drive" "file=$SCRIPT_DIR/data.ext4,format=raw,if=virtio,id=data"

    # Network with port forwarding
    "-netdev" "user,id=net0,hostfwd=tcp::2222-:22,hostfwd=tcp::2323-:23,hostfwd=tcp::8080-:8080,hostfwd=tcp::9100-:9100"
    "-device" "virtio-net-pci,netdev=net0"

    "-no-reboot"
)

# Kernel command line
KCMD="root=/dev/vda rw console=ttyAMA0 loglevel=4"

# Display mode
if [ "$DISPLAY_MODE" = "gui" ]; then
    QEMU_ARGS+=(
        "-device" "virtio-gpu-pci"
        "-device" "qemu-xhci"
        "-device" "usb-kbd"
        "-device" "usb-tablet"
        # Serial output to file so boot logs are visible even in GUI mode.
        # Use chardev+tee so output goes to both file and QEMU's stdout.
        "-chardev" "file,id=serial0,path=$SCRIPT_DIR/serial.log"
        "-serial" "chardev:serial0"
    )
    # Use cocoa on macOS, gtk on Linux
    if [ "$(uname -s)" = "Darwin" ]; then
        QEMU_ARGS+=("-display" "cocoa,show-cursor=on")
    else
        QEMU_ARGS+=("-display" "gtk")
    fi
    # console=tty0 first (display), console=ttyAMA0 last (serial gets all userspace output)
    KCMD="root=/dev/vda rw console=tty0 console=ttyAMA0 loglevel=4"
    echo "Display: graphical window"
    echo "Serial log: $SCRIPT_DIR/serial.log"
else
    QEMU_ARGS+=("-nographic")
fi

QEMU_ARGS+=("-append" "$KCMD")

# 9P host directory sharing
if [ -n "$SHARED_DIR" ] && [ -d "$SHARED_DIR" ]; then
    QEMU_ARGS+=(
        "-virtfs" "local,path=$SHARED_DIR,mount_tag=host0,security_model=mapped-xattr,id=host0"
    )
    echo "Host share: $SHARED_DIR → /mnt/host"
fi

# GDB server
if [ $ENABLE_GDB -eq 1 ]; then
    QEMU_ARGS+=("-s" "-S")
    echo "GDB: localhost:1234 (paused, waiting for debugger)"
fi

echo ""
echo "══════════════════════════════════════════════════════"
echo "  ZaraOS — Buildroot QEMU Environment"
echo "══════════════════════════════════════════════════════"
echo "  SSH:     ssh root@localhost -p 2222"
echo "  Cortex:  curl http://localhost:8080/health"
echo "  Exit:    Ctrl+A X  (or 'poweroff' in VM)"
echo "══════════════════════════════════════════════════════"
echo ""

exec $QEMU "${QEMU_ARGS[@]}" "${EXTRA_ARGS[@]}"
LAUNCHER

chmod +x "${BINARIES_DIR}/run-zaraos-qemu.sh"

# ┌─────────────────────────────────────────────────────────────────┐
# │ SUMMARY                                                         │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "QEMU dev environment ready!"
echo ""
echo "  Kernel:    ${BINARIES_DIR}/Image"
echo "  Rootfs:    ${BINARIES_DIR}/rootfs.ext4"
echo "  Data:      ${BINARIES_DIR}/data.ext4"
echo "  Launcher:  ${BINARIES_DIR}/run-zaraos-qemu.sh"
echo ""
echo "  Run:  ${BINARIES_DIR}/run-zaraos-qemu.sh"
echo "  GUI:  ${BINARIES_DIR}/run-zaraos-qemu.sh --gui"
echo "═══════════════════════════════════════════════════════════════"
