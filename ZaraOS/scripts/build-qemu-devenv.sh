#!/bin/bash
#
# ZaraOS QEMU Full Development Environment Builder
# Creates a production-faithful QEMU VM with all ZaraOS services:
#   - Cortex API server (cross-compiled from Go source)
#   - Container runtime (containerd + nerdctl + CNI)
#   - SSH server (dropbear)
#   - ZaraOS init chain (rcS + all S* scripts)
#   - Graphical display (virtio-gpu, Pi framebuffer emulation)
#   - Networking with port forwarding (SSH:2222, Cortex:8080)
#   - Shared host directory via 9P
#
# Usage: ./build-qemu-devenv.sh [--clean] [--skip-kernel]
#
# Prerequisites: nix (provides Go, e2fsprogs, cross-compiler)

set -eo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
ZARAOS_DIR="$PROJECT_ROOT/ZaraOS"
CORTEX_DIR="$PROJECT_ROOT/cortex"

WORK_DIR="${WORK_DIR:-$PROJECT_ROOT/../kernel-build}"
DEVENV_DIR="$WORK_DIR/devenv"
ROOTFS_DIR="$DEVENV_DIR/rootfs"
OUTPUT_DIR="$WORK_DIR/output"
DOWNLOAD_DIR="$WORK_DIR/downloads"

# Alpine Linux base
ALPINE_VERSION="3.21.3"
ALPINE_ARCH="aarch64"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION%.*}/releases/${ALPINE_ARCH}/alpine-minirootfs-${ALPINE_VERSION}-${ALPINE_ARCH}.tar.gz"

# Container runtime versions (static aarch64 binaries)
CONTAINERD_VERSION="2.0.4"
NERDCTL_VERSION="2.2.2"
CNI_VERSION="1.6.2"
RUNC_VERSION="1.2.5"

# Rootfs image size
ROOTFS_SIZE_MB=2048

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[OK]${NC} $1" >&2; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_step()    { echo -e "\n${CYAN}━━━ $1 ━━━${NC}" >&2; }

# ============================================================================
# Step 1: Cross-compile Cortex API server
# ============================================================================

build_cortex() {
    log_step "Step 1/7: Cross-compiling Cortex API server"

    if [ -f "$DEVENV_DIR/bin/cortex" ]; then
        log_info "Cortex binary already exists, skipping (use --clean to rebuild)"
        return 0
    fi

    mkdir -p "$DEVENV_DIR/bin"

    log_info "Building Cortex for linux/arm64..."
    cd "$CORTEX_DIR"
    nix-shell -p go --run "CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o '$DEVENV_DIR/bin/cortex' ./cmd/"

    local size
    size=$(ls -lh "$DEVENV_DIR/bin/cortex" | awk '{print $5}')
    log_success "Cortex binary: $DEVENV_DIR/bin/cortex ($size)"

    # Also build the CLI
    log_info "Building Cortex CLI for linux/arm64..."
    nix-shell -p go --run "CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o '$DEVENV_DIR/bin/cortex-cli' ./cmd/cli/"
    log_success "Cortex CLI built"
}

# ============================================================================
# Step 2: Download Alpine Linux minirootfs
# ============================================================================

download_alpine() {
    log_step "Step 2/7: Downloading Alpine Linux aarch64 minirootfs"

    local tarball="$DOWNLOAD_DIR/alpine-minirootfs-${ALPINE_VERSION}-${ALPINE_ARCH}.tar.gz"

    if [ -f "$tarball" ]; then
        log_info "Alpine tarball already downloaded"
    else
        mkdir -p "$DOWNLOAD_DIR"
        log_info "Downloading Alpine Linux ${ALPINE_VERSION} (${ALPINE_ARCH})..."
        curl -L -o "$tarball" "$ALPINE_URL" --progress-bar >&2
    fi

    log_success "Alpine base: $tarball"
    echo "$tarball"
}

# ============================================================================
# Step 3: Download container runtime binaries
# ============================================================================

download_container_runtime() {
    log_step "Step 3/7: Downloading container runtime binaries"

    mkdir -p "$DEVENV_DIR/bin" "$DEVENV_DIR/cni"

    # containerd (static binary)
    if [ ! -f "$DEVENV_DIR/bin/containerd" ]; then
        log_info "Downloading containerd ${CONTAINERD_VERSION}..."
        local containerd_url="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-static-${CONTAINERD_VERSION}-linux-arm64.tar.gz"
        curl -L "$containerd_url" --progress-bar >&2 | tar xz -C "$DEVENV_DIR/bin" --strip-components=1
        log_success "containerd downloaded"
    else
        log_info "containerd already downloaded"
    fi

    # runc (static binary)
    if [ ! -f "$DEVENV_DIR/bin/runc" ]; then
        log_info "Downloading runc ${RUNC_VERSION}..."
        curl -L -o "$DEVENV_DIR/bin/runc" \
            "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.arm64" --progress-bar >&2
        chmod +x "$DEVENV_DIR/bin/runc"
        log_success "runc downloaded"
    else
        log_info "runc already downloaded"
    fi

    # nerdctl (static binary)
    if [ ! -f "$DEVENV_DIR/bin/nerdctl" ]; then
        log_info "Downloading nerdctl ${NERDCTL_VERSION}..."
        local nerdctl_url="https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-arm64.tar.gz"
        curl -L "$nerdctl_url" --progress-bar >&2 | tar xz -C "$DEVENV_DIR/bin" nerdctl
        log_success "nerdctl downloaded"
    else
        log_info "nerdctl already downloaded"
    fi

    # CNI plugins
    if [ ! -f "$DEVENV_DIR/cni/bridge" ]; then
        log_info "Downloading CNI plugins ${CNI_VERSION}..."
        mkdir -p "$DEVENV_DIR/cni"
        local cni_url="https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-arm64-v${CNI_VERSION}.tgz"
        curl -L "$cni_url" --progress-bar >&2 | tar xz -C "$DEVENV_DIR/cni"
        log_success "CNI plugins downloaded"
    else
        log_info "CNI plugins already downloaded"
    fi

    log_success "Container runtime binaries ready"
}

# ============================================================================
# Step 4: Download SSH server (dropbear - lightweight)
# ============================================================================

download_ssh() {
    log_step "Step 4/7: SSH server setup"

    # We'll use Alpine's dropbear which will be available in the rootfs
    # via apk. For now just note it will be configured.
    log_info "SSH will be configured via Alpine's dropbear package inside the VM"
    log_success "SSH setup planned"
}

# ============================================================================
# Step 5: Assemble the rootfs directory tree
# ============================================================================

assemble_rootfs() {
    log_step "Step 5/7: Assembling rootfs"

    local alpine_tarball="$1"

    # Start fresh
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR"

    # --- Extract Alpine base ---
    log_info "Extracting Alpine base..."
    tar xzf "$alpine_tarball" -C "$ROOTFS_DIR"

    # --- Create ZaraOS directory structure ---
    log_info "Creating ZaraOS directory structure..."
    mkdir -p "$ROOTFS_DIR"/{boot,data,mnt/host,run,tmp}
    mkdir -p "$ROOTFS_DIR"/var/lib/containerd
    mkdir -p "$ROOTFS_DIR"/etc/cni/net.d
    mkdir -p "$ROOTFS_DIR"/opt/cni/bin
    mkdir -p "$ROOTFS_DIR"/etc/init.d
    mkdir -p "$ROOTFS_DIR"/etc/network
    mkdir -p "$ROOTFS_DIR"/etc/modules-load.d
    mkdir -p "$ROOTFS_DIR"/etc/dropbear
    mkdir -p "$ROOTFS_DIR"/usr/bin
    mkdir -p "$ROOTFS_DIR"/usr/sbin
    mkdir -p "$ROOTFS_DIR"/var/log

    # --- Copy ZaraOS overlays ---
    log_info "Applying ZaraOS overlay files..."
    local overlay_dir="$ZARAOS_DIR/overlays"

    # Copy etc files
    cp "$overlay_dir/etc/hostname" "$ROOTFS_DIR/etc/hostname" 2>/dev/null || true
    cp "$overlay_dir/etc/motd" "$ROOTFS_DIR/etc/motd" 2>/dev/null || true
    cp "$overlay_dir/etc/zaraos-release" "$ROOTFS_DIR/etc/zaraos-release" 2>/dev/null || true

    # Copy init scripts (we'll adapt them for QEMU)
    for script in "$overlay_dir"/etc/init.d/S*; do
        [ -f "$script" ] || continue
        cp "$script" "$ROOTFS_DIR/etc/init.d/"
        chmod +x "$ROOTFS_DIR/etc/init.d/$(basename "$script")"
    done

    # Copy utility scripts
    for util in "$overlay_dir"/usr/bin/*; do
        [ -f "$util" ] || continue
        cp "$util" "$ROOTFS_DIR/usr/bin/"
        chmod +x "$ROOTFS_DIR/usr/bin/$(basename "$util")"
    done

    # --- Install Cortex binary ---
    log_info "Installing Cortex API server..."
    cp "$DEVENV_DIR/bin/cortex" "$ROOTFS_DIR/usr/bin/cortex"
    chmod +x "$ROOTFS_DIR/usr/bin/cortex"
    if [ -f "$DEVENV_DIR/bin/cortex-cli" ]; then
        cp "$DEVENV_DIR/bin/cortex-cli" "$ROOTFS_DIR/usr/bin/cortex-cli"
        chmod +x "$ROOTFS_DIR/usr/bin/cortex-cli"
    fi

    # --- Install container runtime ---
    log_info "Installing container runtime..."
    for bin in containerd containerd-shim-runc-v2 ctr runc nerdctl; do
        if [ -f "$DEVENV_DIR/bin/$bin" ]; then
            cp "$DEVENV_DIR/bin/$bin" "$ROOTFS_DIR/usr/bin/$bin"
            chmod +x "$ROOTFS_DIR/usr/bin/$bin"
        fi
    done

    # Install CNI plugins
    cp "$DEVENV_DIR/cni/"* "$ROOTFS_DIR/opt/cni/bin/" 2>/dev/null || true
    chmod +x "$ROOTFS_DIR/opt/cni/bin/"* 2>/dev/null || true

    # --- Create CNI default config ---
    cat > "$ROOTFS_DIR/etc/cni/net.d/10-bridge.conflist" << 'EOF'
{
  "cniVersion": "1.0.0",
  "name": "bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": { "type": "host-local", "ranges": [[{"subnet": "10.88.0.0/16"}]], "routes": [{"dst": "0.0.0.0/0"}] }
    },
    { "type": "portmap", "capabilities": {"portMappings": true} },
    { "type": "firewall" },
    { "type": "tuning" }
  ]
}
EOF

    # --- Create containerd config ---
    mkdir -p "$ROOTFS_DIR/etc/containerd"
    cat > "$ROOTFS_DIR/etc/containerd/config.toml" << 'EOF'
version = 3

[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc.options]
  SystemdCgroup = false
[plugins."io.containerd.cri.v1.runtime".cni]
  bin_dir = "/opt/cni/bin"
  conf_dir = "/etc/cni/net.d"
EOF

    # --- Create QEMU-adapted rcS init script ---
    log_info "Creating QEMU-adapted init scripts..."
    cat > "$ROOTFS_DIR/etc/init.d/rcS" << 'RCSINIT'
#!/bin/sh
# ===================================================================
# ZaraOS QEMU Development Environment - Init Script
# Mirrors production rcS but adapted for QEMU virt machine
# ===================================================================

echo ""
echo '$$$$$$$$\                              $$$$$$\   $$$$$$\ '
echo '\____$$  |                            $$  __$$\ $$  __$$\'
echo '    $$  / $$$$$$\   $$$$$$\  $$$$$$\  $$ /  $$ |$$ /  \__|'
echo '   $$  /  \____$$\ $$  __$$\ \____$$\ $$ |  $$ |\$$$$$$\ '
echo '  $$  /   $$$$$$$ |$$ |  \__|$$$$$$$ |$$ |  $$ | \____$$\ '
echo ' $$  /   $$  __$$ |$$ |     $$  __$$ |$$ |  $$ |$$\   $$ |'
echo '$$$$$$$$\\$$$$$$$ |$$ |     \$$$$$$$ | $$$$$$  |\$$$$$$  |'
echo '\________|\_______|\__|      \_______| \______/  \______/ '
echo ""
echo "  QEMU Development Environment"
echo ""

export PATH=/usr/bin:/usr/sbin:/bin:/sbin:/opt/cni/bin

# Mount essential filesystems
echo "[init] Mounting filesystems..."
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mount -o remount,rw /
[ -d /dev/pts ] || mkdir -p /dev/pts
mount -t devpts devpts /dev/pts
mount -t tmpfs tmpfs /tmp
mount -t tmpfs tmpfs /run

# Cgroups v2 (required for containers)
echo "[init] Setting up cgroups..."
mkdir -p /sys/fs/cgroup
mount -t cgroup2 cgroup2 /sys/fs/cgroup 2>/dev/null || \
    echo "[init] WARNING: cgroup2 mount failed"

# Hostname
[ -f /etc/hostname ] && hostname -F /etc/hostname
echo "[init] Hostname: $(hostname)"

# Kernel logging
[ -x /sbin/klogd ] && klogd
[ -x /sbin/syslogd ] && syslogd

# Network (QEMU user-mode networking provides DHCP)
echo "[init] Configuring network..."
ip link set lo up
ip link set eth0 up 2>/dev/null

# Use udhcpc (BusyBox DHCP client) for QEMU user-mode networking
if command -v udhcpc >/dev/null 2>&1; then
    udhcpc -i eth0 -b -q 2>/dev/null &
fi

# Wait briefly for network
sleep 2

echo "[init] Network:"
ip -4 addr show eth0 2>/dev/null | grep inet | sed 's/^/  /'

# Mount persistent data disk if available (/dev/vdb)
echo "[init] Mounting data partition..."
if [ -b /dev/vdb ]; then
    # Format on first use (mke2fs is the low-level tool, always available)
    if ! blkid /dev/vdb 2>/dev/null | grep -q ext4; then
        echo "[init] First boot: formatting data partition..."
        if command -v mkfs.ext4 >/dev/null 2>&1; then
            mkfs.ext4 -q -L zaraos-data /dev/vdb
        elif command -v mke2fs >/dev/null 2>&1; then
            mke2fs -t ext4 -q -L zaraos-data /dev/vdb
        else
            # BusyBox mke2fs fallback (ext2 only, but works)
            mke2fs -L zaraos-data /dev/vdb 2>/dev/null || \
                echo "[init] WARNING: cannot format data disk (no mke2fs)"
        fi
    fi
    mkdir -p /data
    mount /dev/vdb /data 2>/dev/null || mount -t ext4 /dev/vdb /data 2>/dev/null || \
        mount -t ext2 /dev/vdb /data 2>/dev/null || \
        echo "[init] WARNING: could not mount data disk"
    mkdir -p /data/containerd /data/cni
    # Bind-mount for containerd
    mkdir -p /var/lib/containerd
    mount --bind /data/containerd /var/lib/containerd
    echo "[init] Data partition mounted at /data"
else
    echo "[init] No data disk found (add -drive for persistence)"
    mkdir -p /data /var/lib/containerd
fi

# Mount 9P host share if available
if grep -q 9p /proc/filesystems 2>/dev/null; then
    mkdir -p /mnt/host
    mount -t 9p -o trans=virtio host0 /mnt/host 2>/dev/null && \
        echo "[init] Host share: /mnt/host" || true
fi

# Auto-install packages on first boot (requires network)
if [ ! -f /etc/.packages-installed ]; then
    echo "[init] First boot: installing packages via apk..."
    if [ -f /etc/apk/repositories ]; then
        apk update --quiet 2>/dev/null
        apk add --no-cache --quiet \
            dropbear dropbear-scp \
            e2fsprogs e2fsprogs-extra \
            htop curl iptables ip6tables socat \
            bash procps coreutils shadow \
            util-linux iproute2 \
            2>/dev/null && {
            echo "[init] Packages installed successfully"
            # Set root password for SSH
            echo "root:zaraos" | chpasswd 2>/dev/null
            touch /etc/.packages-installed
        } || echo "[init] WARNING: package install failed (network?)"
    fi
fi

# Re-format data disk now that we have e2fsprogs
if [ -b /dev/vdb ] && ! mount | grep -q "/data"; then
    if command -v mkfs.ext4 >/dev/null 2>&1; then
        if ! blkid /dev/vdb 2>/dev/null | grep -q ext4; then
            echo "[init] Formatting data partition with ext4..."
            mkfs.ext4 -q -L zaraos-data /dev/vdb 2>/dev/null
        fi
        mkdir -p /data
        mount -t ext4 /dev/vdb /data 2>/dev/null && {
            mkdir -p /data/containerd /data/cni
            mkdir -p /var/lib/containerd
            mount --bind /data/containerd /var/lib/containerd 2>/dev/null
            echo "[init] Data partition mounted at /data"
        }
    fi
fi

# Start SSH server (dropbear)
echo "[init] Starting SSH server..."
if command -v dropbear >/dev/null 2>&1; then
    # Generate host keys if needed
    mkdir -p /etc/dropbear
    [ -f /etc/dropbear/dropbear_rsa_host_key ] || \
        dropbearkey -t rsa -f /etc/dropbear/dropbear_rsa_host_key 2>/dev/null
    [ -f /etc/dropbear/dropbear_ed25519_host_key ] || \
        dropbearkey -t ed25519 -f /etc/dropbear/dropbear_ed25519_host_key 2>/dev/null
    dropbear -R -B 2>/dev/null
    echo "[init] SSH: listening on port 22 (host:2222)"
else
    echo "[init] WARNING: dropbear not found, SSH unavailable"
    echo "[init]   Fix: run 'setup-packages' after boot"
fi

# Start containerd
echo "[init] Starting containerd..."
if command -v containerd >/dev/null 2>&1; then
    containerd --config /etc/containerd/config.toml </dev/null &>/var/log/containerd.log &
    # Wait for socket
    timeout=10
    while [ ! -S /run/containerd/containerd.sock ] && [ $timeout -gt 0 ]; do
        sleep 1
        timeout=$((timeout - 1))
    done
    if [ -S /run/containerd/containerd.sock ]; then
        echo "[init] containerd: running"
    else
        echo "[init] WARNING: containerd failed to start (check /var/log/containerd.log)"
    fi
else
    echo "[init] WARNING: containerd not found"
fi

# Run ZaraOS S* init scripts (skip hardware-specific ones)
echo "[init] Running service scripts..."
for script in /etc/init.d/S*; do
    [ -x "$script" ] || continue
    name="$(basename "$script")"
    # Skip hardware-specific scripts in QEMU
    case "$name" in
        S01-mounts*)  echo "[init]   skip $name (QEMU: no mmcblk)" ;;
        S02-first*)   echo "[init]   skip $name (QEMU: no partition resize)" ;;
        S40-blue*)    echo "[init]   skip $name (QEMU: no bluetooth)" ;;
        S30-dbus*)    echo "[init]   skip $name (not installed in QEMU)" ;;
        S50-node*)    echo "[init]   skip $name (not installed)" ;;
        S60-cortex*)  echo "[init]   skip $name (started by rcS directly)" ;;
        *)
            echo "[init]   running $name..."
            "$script" start </dev/null 2>&1 || echo "[init]   WARNING: $name failed"
            ;;
    esac
done

# Start Cortex API server (if not already started by S60-cortex.sh)
echo "[init] Starting Cortex API server..."
if pgrep -x cortex >/dev/null 2>&1; then
    echo "[init] Cortex: already running on :8080 (host:8080)"
elif command -v cortex >/dev/null 2>&1; then
    GITHUB_ORG=KoalbyMQP \
    GITHUB_REPOS=Core \
    DOCKERHUB_ORG=koalby \
    cortex </dev/null &>/var/log/cortex.log &
    CORTEX_PID=$!
    sleep 2
    if kill -0 $CORTEX_PID 2>/dev/null; then
        echo "[init] Cortex: running on :8080 (host:8080)"
    else
        echo "[init] WARNING: Cortex failed to start (check /var/log/cortex.log)"
    fi
else
    echo "[init] WARNING: cortex binary not found"
fi

# Print status summary
echo ""
echo "  ╔══════════════════════════════════════════════════════╗"
echo "  ║        ZaraOS QEMU Dev Environment Ready             ║"
echo "  ╚══════════════════════════════════════════════════════╝"
echo ""
echo "  Kernel:    $(uname -r)"
echo "  Hostname:  $(hostname)"
echo "  Arch:      $(uname -m)"
echo ""
echo "  Services:"
[ -S /run/containerd/containerd.sock ] && echo "    ✓ containerd" || echo "    ✗ containerd"
pgrep -x cortex >/dev/null && echo "    ✓ cortex (port 8080)" || echo "    ✗ cortex"
pgrep -x dropbear >/dev/null && echo "    ✓ SSH (port 22 → host:2222)" || echo "    ✗ SSH"
echo ""
echo "  From host machine:"
echo "    ssh -p 2222 root@localhost"
echo "    curl http://localhost:8080/health"
echo "    nerdctl ps"
echo ""
echo "  Host share: /mnt/host"
echo "  Data disk:  /data"
echo ""
echo "  Exit: poweroff  (or Ctrl+A then X)"
echo ""

RCSINIT

    chmod +x "$ROOTFS_DIR/etc/init.d/rcS"

    # --- Create inittab (mirrors production) ---
    cat > "$ROOTFS_DIR/etc/inittab" << 'EOF'
# ZaraOS QEMU inittab
::sysinit:/etc/init.d/rcS

# Serial console (QEMU -nographic)
ttyAMA0::respawn:/bin/sh -l

# Graphical console (QEMU -display cocoa)
tty1::respawn:/sbin/getty -n -l /usr/bin/autologin.sh 0 tty1 linux

::ctrlaltdel:/sbin/reboot
::shutdown:/bin/umount -a -r
EOF

    # --- Create autologin script ---
    cat > "$ROOTFS_DIR/usr/bin/autologin.sh" << 'EOF'
#!/bin/sh
exec /bin/login -f root
EOF
    chmod +x "$ROOTFS_DIR/usr/bin/autologin.sh"

    # --- Create a setup-packages helper script ---
    # This runs inside the VM to install packages via apk
    cat > "$ROOTFS_DIR/usr/bin/setup-packages" << 'SETUP'
#!/bin/sh
# One-time package setup - installs needed packages via Alpine apk
# Run this after first boot if SSH/tools are missing

echo "Setting up Alpine packages..."
echo "https://dl-cdn.alpinelinux.org/alpine/v3.21/main" > /etc/apk/repositories
echo "https://dl-cdn.alpinelinux.org/alpine/v3.21/community" >> /etc/apk/repositories

apk update
apk add --no-cache \
    dropbear \
    dropbear-scp \
    htop \
    curl \
    iptables \
    ip6tables \
    socat \
    strace \
    tmux \
    e2fsprogs \
    e2fsprogs-extra \
    util-linux \
    bash \
    procps \
    coreutils \
    shadow \
    net-tools \
    iproute2

# Enable root login with empty password for dev
echo "root:zaraos" | chpasswd
passwd -u root

# Restart dropbear
mkdir -p /etc/dropbear
[ -f /etc/dropbear/dropbear_rsa_host_key ] || \
    dropbearkey -t rsa -f /etc/dropbear/dropbear_rsa_host_key
[ -f /etc/dropbear/dropbear_ed25519_host_key ] || \
    dropbearkey -t ed25519 -f /etc/dropbear/dropbear_ed25519_host_key
killall dropbear 2>/dev/null
dropbear -R -B

echo ""
echo "Setup complete! SSH is now available."
echo "  From host: ssh -p 2222 root@localhost"
echo "  Password:  zaraos"
echo ""
SETUP
    chmod +x "$ROOTFS_DIR/usr/bin/setup-packages"

    # --- Configure Alpine repos so apk works out of the box ---
    mkdir -p "$ROOTFS_DIR/etc/apk"
    cat > "$ROOTFS_DIR/etc/apk/repositories" << 'EOF'
https://dl-cdn.alpinelinux.org/alpine/v3.21/main
https://dl-cdn.alpinelinux.org/alpine/v3.21/community
EOF

    # --- Configure root user for dev ---
    # Set empty root password for dev (login without password)
    sed -i 's|^root:.*|root::0:0:root:/root:/bin/sh|' "$ROOTFS_DIR/etc/passwd" 2>/dev/null || true

    # --- DNS resolution ---
    cat > "$ROOTFS_DIR/etc/resolv.conf" << 'EOF'
nameserver 10.0.2.3
EOF

    # --- Profile for interactive shells ---
    cat > "$ROOTFS_DIR/etc/profile" << 'PROFILE'
export PATH=/usr/bin:/usr/sbin:/bin:/sbin:/opt/cni/bin
export HOME=/root
export TERM=linux
export PS1='\[\033[1;32m\]zaraos\[\033[0m\]:\[\033[1;34m\]\w\[\033[0m\]\$ '

alias ll='ls -la'
alias nerdctl='nerdctl --cni-path=/opt/cni/bin --cni-netconfpath=/etc/cni/net.d'

# Show status on login
echo ""
echo "ZaraOS $(cat /etc/zaraos-release 2>/dev/null | head -1 || echo 'Dev')"
echo "Kernel: $(uname -r) | Arch: $(uname -m)"
echo ""
PROFILE

    log_success "Rootfs assembled: $(du -sh "$ROOTFS_DIR" | awk '{print $1}')"
}

# ============================================================================
# Step 6: Create ext4 disk image
# ============================================================================

create_rootfs_image() {
    log_step "Step 6/7: Creating ext4 rootfs image"

    local image="$OUTPUT_DIR/rootfs-devenv.ext4"

    log_info "Creating ${ROOTFS_SIZE_MB}MB ext4 image..."

    # Create the ext4 image using nix-provided e2fsprogs
    # We use mke2fs to create the filesystem directly from a directory
    nix-shell -p e2fsprogs --run "
        # Create empty file
        dd if=/dev/zero of='$image' bs=1M count=$ROOTFS_SIZE_MB status=progress 2>&1

        # Create ext4 filesystem populated from the rootfs directory
        mke2fs -t ext4 -d '$ROOTFS_DIR' -L zaraos-root \
            -b 4096 -m 1 -O ^metadata_csum \
            '$image' 2>&1
    "

    local size
    size=$(ls -lh "$image" | awk '{print $5}')
    log_success "Rootfs image: $image ($size)"
}

# ============================================================================
# Step 7: Create data disk & QEMU launch script
# ============================================================================

create_launch_script() {
    log_step "Step 7/7: Creating QEMU launch script"

    local data_disk="$OUTPUT_DIR/data-devenv.qcow2"

    # Create persistent data disk (sparse, grows on demand)
    if [ ! -f "$data_disk" ]; then
        log_info "Creating persistent data disk (4GB sparse)..."
        /opt/homebrew/bin/qemu-img create -f qcow2 "$data_disk" 4G >&2
    fi

    # Create the launch script
    cat > "$OUTPUT_DIR/run-devenv.sh" << 'LAUNCHER'
#!/bin/bash
# ZaraOS QEMU Development Environment Launcher
#
# Usage: ./run-devenv.sh [--gui] [--shared-dir DIR] [--gdb]
#
# Access:
#   SSH:      ssh -p 2222 root@localhost  (password: zaraos)
#   Cortex:   curl http://localhost:8080/health
#   Display:  Pass --gui for graphical window

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Defaults
DISPLAY_MODE="nographic"
SHARED_DIR=""
ENABLE_GDB=0
EXTRA_ARGS=()

# Parse args
while [[ $# -gt 0 ]]; do
    case $1 in
        --gui)       DISPLAY_MODE="gui" ;;
        --shared-dir) SHARED_DIR="$2"; shift ;;
        --gdb)       ENABLE_GDB=1 ;;
        *)           EXTRA_ARGS+=("$1") ;;
    esac
    shift
done

QEMU="/opt/homebrew/bin/qemu-system-aarch64"
if ! command -v "$QEMU" &>/dev/null; then
    QEMU="qemu-system-aarch64"
fi

# Detect CPU type
CPU="cortex-a76"
if ! $QEMU -cpu help 2>/dev/null | grep -q "$CPU"; then
    CPU="max"
fi

# Build command
QEMU_ARGS=(
    "-M" "virt"
    "-cpu" "$CPU"
    "-m" "4G"
    "-smp" "4"
    "-kernel" "$SCRIPT_DIR/Image"

    # Root filesystem
    "-drive" "file=$SCRIPT_DIR/rootfs-devenv.ext4,format=raw,if=virtio,id=rootfs"

    # Persistent data disk
    "-drive" "file=$SCRIPT_DIR/data-devenv.qcow2,format=qcow2,if=virtio,id=data"

    # Network with port forwarding
    "-netdev" "user,id=net0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:8080,hostfwd=tcp::9100-:9100"
    "-device" "virtio-net-pci,netdev=net0"

    "-no-reboot"
)

# Kernel command line
KCMD="root=/dev/vda rw console=ttyAMA0 loglevel=4"

# Display mode
if [ "$DISPLAY_MODE" = "gui" ]; then
    QEMU_ARGS+=(
        "-device" "virtio-gpu-pci"
        "-display" "cocoa,show-cursor=on"
        "-device" "qemu-xhci"
        "-device" "usb-kbd"
        "-device" "usb-tablet"
    )
    KCMD="$KCMD console=tty0"
    echo "Starting with GUI display (Cocoa window)..."
else
    QEMU_ARGS+=("-nographic")
    echo "Starting in terminal mode (use --gui for graphical window)..."
fi

QEMU_ARGS+=("-append" "$KCMD")

# 9P shared directory
if [ -n "$SHARED_DIR" ] && [ -d "$SHARED_DIR" ]; then
    QEMU_ARGS+=(
        "-virtfs" "local,path=$SHARED_DIR,mount_tag=host0,security_model=mapped-xattr,id=host0"
    )
    echo "Sharing: $SHARED_DIR → /mnt/host"
fi

# GDB
if [ $ENABLE_GDB -eq 1 ]; then
    QEMU_ARGS+=("-s" "-S")
    echo "GDB server on localhost:1234 (paused)"
fi

echo ""
echo "══════════════════════════════════════════════════════════"
echo "  ZaraOS QEMU Dev Environment"
echo "══════════════════════════════════════════════════════════"
echo "  SSH:     ssh -p 2222 root@localhost (password: zaraos)"
echo "  Cortex:  curl http://localhost:8080/health"
echo "  Exit:    Ctrl+A X  (or 'poweroff' in VM)"
echo "══════════════════════════════════════════════════════════"
echo ""

exec $QEMU "${QEMU_ARGS[@]}" "${EXTRA_ARGS[@]}"
LAUNCHER

    chmod +x "$OUTPUT_DIR/run-devenv.sh"

    log_success "Launch script: $OUTPUT_DIR/run-devenv.sh"
}

# ============================================================================
# Main
# ============================================================================

main() {
    local skip_kernel=0

    while [[ $# -gt 0 ]]; do
        case $1 in
            --clean)
                log_warn "Cleaning dev environment..."
                rm -rf "$DEVENV_DIR" "$OUTPUT_DIR/rootfs-devenv.ext4" "$OUTPUT_DIR/data-devenv.qcow2" "$OUTPUT_DIR/run-devenv.sh"
                log_success "Cleaned"
                exit 0
                ;;
            --skip-kernel) skip_kernel=1 ;;
            --help)
                echo "Usage: $0 [--clean] [--skip-kernel]"
                exit 0
                ;;
        esac
        shift
    done

    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   ZaraOS QEMU Full Dev Environment Builder                  ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Check kernel image exists
    if [ ! -f "$OUTPUT_DIR/Image" ]; then
        log_error "Kernel image not found at $OUTPUT_DIR/Image"
        log_error "Build kernel first: ./build-kernel-local.sh --qemu"
        exit 1
    fi

    mkdir -p "$DEVENV_DIR" "$OUTPUT_DIR" "$DOWNLOAD_DIR"

    # Execute build pipeline
    build_cortex

    local alpine_tarball
    alpine_tarball=$(download_alpine)

    download_container_runtime
    download_ssh
    assemble_rootfs "$alpine_tarball"
    create_rootfs_image
    create_launch_script

    # Resolve to absolute path for clarity
    local abs_output
    abs_output="$(cd "$OUTPUT_DIR" && pwd)"

    echo ""
    log_success "Build complete!"
    echo ""
    echo -e "  ${GREEN}Start the dev environment:${NC}"
    echo ""
    echo "    # Terminal mode (serial console)"
    echo "    $abs_output/run-devenv.sh"
    echo ""
    echo "    # GUI mode (graphical window like Pi HDMI)"
    echo "    $abs_output/run-devenv.sh --gui"
    echo ""
    echo "    # With host directory sharing"
    echo "    $abs_output/run-devenv.sh --shared-dir /path/to/project"
    echo ""
    echo -e "  ${GREEN}First boot:${NC} run 'setup-packages' inside VM to install SSH + tools"
    echo ""
}

main "$@"
