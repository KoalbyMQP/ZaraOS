# ZaraOS Kernel Build & Testing Guide

This guide walks you through building the ZaraOS kernel locally and testing it in a QEMU VM or on actual Raspberry Pi 5 hardware.

## Quick Start

```bash
cd ZaraOS/scripts

# Build kernel for QEMU and boot it in a VM (recommended first step)
./build-kernel-local.sh --qemu
./qemu-test-kernel.sh

# Or use the workflow shortcut (does both in one command)
./kernel-workflow.sh qemu
```

## Prerequisites

### macOS
```bash
# Install build tools
brew install make gcc@14 bison flex

# Install cross-compiler (required for both Pi 5 and QEMU builds)
brew tap messense/macos-cross-toolchains
brew install aarch64-unknown-linux-gnu

# Install QEMU for local testing
brew install qemu
```

### Linux (Ubuntu/Debian)
```bash
# Install build tools and cross-compiler
sudo apt install build-essential bison flex libssl-dev gcc-aarch64-linux-gnu

# Install QEMU
sudo apt install qemu-system-arm
```

## Building the Kernel

### For QEMU Testing (Recommended)
```bash
cd ZaraOS/scripts
./build-kernel-local.sh --qemu
```

**What this does:**
1. Downloads the Raspberry Pi Linux kernel (stable_20250916)
2. Applies QEMU-optimized configuration (generic arm64 defconfig + `linux-qemu.fragment`)
3. Compiles kernel with 4K pages and virtio drivers
4. Downloads a static BusyBox binary and creates an initramfs
5. Output is ready to boot in QEMU immediately

**Key differences from Pi 5 build:**
- Uses generic `defconfig` instead of `bcm2712_defconfig`
- 4K page size (QEMU `virt` requires 4K; Pi 5 uses 16K)
- Virtio drivers (block, net, console) instead of Pi 5 hardware
- PL011 UART for serial console
- 9P filesystem support for host directory sharing
- No Pi 5 GPU (VC4/V3D), WiFi (Broadcom), or PWM drivers

### For Raspberry Pi 5
```bash
cd ZaraOS/scripts
./build-kernel-local.sh
```

**What this does:**
1. Downloads the Raspberry Pi Linux kernel (stable_20250916)
2. Applies Pi 5 configuration (`bcm2712_defconfig` + `linux-pi5.fragment`)
3. Compiles kernel, modules, and copies device trees
4. Creates a deployment-ready archive

### Build Options

```bash
# Use 8 parallel jobs (faster on multi-core)
./build-kernel-local.sh -j 8

# Interactive kernel configuration (menuconfig)
./build-kernel-local.sh --menuconfig

# Clean all previous builds
./build-kernel-local.sh --clean

# QEMU build with menuconfig
./build-kernel-local.sh --qemu --menuconfig -j 8
```

### Output Files

**QEMU build** (`--qemu`):
```
kernel-build/output/
├── Image               # Kernel image
├── initramfs.cpio.gz   # BusyBox initramfs (auto-created)
└── build.log           # Build log
```

**Pi 5 build** (default):
```
kernel-build/output/
├── Image                       # Kernel image (copy to Pi /boot/)
├── dtb/                        # Device tree files
├── modules/                    # Compiled kernel modules
└── zaraos-kernel-*.tar.gz      # Ready-to-deploy archive
```

**Build time estimates:**
- First build: 5-15 minutes (depends on CPU cores)
- Rebuild with ccache: 2-3 minutes

## Testing in QEMU

### Basic Usage

```bash
cd ZaraOS/scripts

# Auto-detects kernel and initramfs from build output
./qemu-test-kernel.sh
```

### Share a Host Directory

You can share a directory from your host machine into the QEMU VM using 9P:

```bash
# Share current project directory
./qemu-test-kernel.sh --shared-dir /path/to/project

# Inside the VM, access it at:
ls /mnt/host
```

### Advanced Options

```bash
# Use more RAM and CPUs
./qemu-test-kernel.sh --memory 8 --cpus 4

# Use custom kernel
./qemu-test-kernel.sh --kernel ./path/to/Image --initramfs ./path/to/initramfs.cpio.gz

# Enable GDB debugging (pauses at boot, connect with gdb-multiarch)
./qemu-test-kernel.sh --gdb
```

### QEMU Keyboard Controls
- **Ctrl+A X** -- Exit QEMU
- **Ctrl+A C** -- Toggle QEMU monitor / serial console
- **Ctrl+A H** -- Show QEMU help

### What to Check in QEMU
- Kernel boots successfully (watch for panic messages)
- Basic commands work (`uname -a`, `ls`, `dmesg`)
- Container support (`cat /proc/cgroups`)
- Network (`ip link show`)
- 9P share mount (if using `--shared-dir`)

## Deploying to Raspberry Pi 5

Once kernel passes QEMU testing, deploy to physical hardware.

### Method 1: Workflow Script (Recommended)

```bash
./kernel-workflow.sh deploy
```

This automatically transfers the kernel archive over SSH and installs it.

### Method 2: Manual SSH Transfer

**On your build machine:**
```bash
cd ZaraOS/scripts
./build-kernel-local.sh
scp ../kernel-build/output/zaraos-kernel-*.tar.gz pi@zaraos:/tmp/
```

**On the Raspberry Pi 5:**
```bash
ssh pi@zaraos

# Extract and backup current kernel
cd /tmp
tar xzf zaraos-kernel-*.tar.gz
sudo cp /boot/Image /boot/Image.backup

# Install new kernel
sudo cp Image /boot/
sudo cp dtb/* /boot/overlays/

# Install modules
sudo cp -r modules/lib/modules/* /lib/modules/
sudo depmod -a

# Reboot
sudo reboot
```

### Method 3: SD Card Modification

1. Build kernel on your machine
2. Mount Pi's SD card on your computer
3. Copy files to boot partition:
   ```bash
   sudo cp Image /Volumes/boot/
   sudo cp dtb/*.dtb /Volumes/boot/overlays/
   ```

### Verifying Installation

After reboot:
```bash
ssh pi@zaraos
uname -a       # Check kernel version
lsmod          # Check modules loaded
dmesg | head -30  # Watch boot log for errors
```

## Kernel Configuration

### Pi 5 Fragment (`kernel-fragments/linux-pi5.fragment`)
- 16K page size (Pi 5 optimization)
- GPU: VC4/V3D DRM drivers
- WiFi: Broadcom brcmfmac
- I/O: GPIO, I2C, SPI, PWM, USB serial
- Container support (namespaces, cgroups, overlay FS, seccomp)

### QEMU Fragment (`kernel-fragments/linux-qemu.fragment`)
- 4K page size (required by QEMU virt)
- Virtio: block, net, console, GPU, input
- PL011 UART serial console
- 9P filesystem for host sharing
- Initramfs + devtmpfs auto-mount
- Container support (same as Pi 5)
- Pi 5 hardware drivers disabled

### Customizing Configuration

```bash
# Launch menuconfig before building
./build-kernel-local.sh --menuconfig

# Your config is saved to output/kernel-config-<timestamp>
```

## Workflow Script

The `kernel-workflow.sh` orchestrates common workflows:

```bash
./kernel-workflow.sh          # Interactive menu
./kernel-workflow.sh qemu     # Build + test in QEMU (fastest iteration)
./kernel-workflow.sh build    # Build for Pi 5
./kernel-workflow.sh test     # Test existing build in QEMU
./kernel-workflow.sh deploy   # Deploy to Pi 5 over SSH
./kernel-workflow.sh full     # Build + test + deploy (Pi 5)
./kernel-workflow.sh status   # Show build output status
```

## Troubleshooting

### Build Fails: "Cross-compiler not found"
```bash
# macOS
brew tap messense/macos-cross-toolchains
brew install aarch64-unknown-linux-gnu

# Linux
sudo apt install gcc-aarch64-linux-gnu

# Or set a custom prefix
export CROSS_COMPILE=aarch64-unknown-linux-gnu-
./build-kernel-local.sh --qemu
```

### QEMU Won't Start
```bash
# Verify QEMU installation
qemu-system-aarch64 --version

# Install if missing
# macOS: brew install qemu
# Linux: sudo apt install qemu-system-arm
```

### Kernel Panic in QEMU
- Check that you built with `--qemu` flag (not the Pi 5 config)
- A 16K page kernel will NOT boot in QEMU — rebuild with `--qemu`
- Check build log: `cat ../kernel-build/output/build.log | tail -50`

### Kernel Panic on Pi Boot
```bash
# Check dmesg for clues
dmesg | grep -i error

# Recovery: restore backup
sudo cp /boot/Image.backup /boot/Image
sudo reboot
```

### Slow Builds
```bash
# Install ccache (used automatically if available)
brew install ccache  # macOS
sudo apt install ccache  # Linux

# Use all CPU cores
./build-kernel-local.sh --qemu -j $(nproc)
```

## Further Reading

- [Buildroot Manual](https://buildroot.org/downloads/manual/manual.html)
- [Raspberry Pi Kernel Build](https://www.raspberrypi.com/documentation/computers/linux_kernel.html)
- [QEMU ARM Emulation](https://wiki.qemu.org/Documentation/Platforms/ARM)
- [Linux Kernel Configuration](https://www.kernel.org/doc/html/latest/admin-guide/index.html)
