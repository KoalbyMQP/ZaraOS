# ZaraOS - Raspberry Pi Custom Linux Distribution

ZaraOS is a specialized Linux distribution built on Buildroot, specifically designed for the Humanoid Robots MQP project. It provides a minimal, optimized operating system tailored for Raspberry Pi hardware with ML workload capabilities.

## Documentation Links
- **Buildroot Manual**: https://buildroot.org/downloads/manual/manual.html
- **Raspberry Pi Documentation**: https://www.raspberrypi.com/documentation/
- **External Trees Guide**: https://buildroot.org/downloads/manual/manual.html#outside-br-custom
- **Buildroot Training**: https://bootlin.com/doc/training/buildroot/

## Supported Raspberry Pi Models

ZaraOS supports the following Raspberry Pi models:
- Raspberry Pi 5 B and 500 (primary target)
- Raspberry Pi 4 B, 400, CM4 and CM4s  
- Raspberry Pi 3 B, B+, CM3 and CM3+
- Raspberry Pi Zero 2 W
- Raspberry Pi Zero W
- Raspberry Pi CM4 IO Board

## Quick Start

### Prerequisites
Install required packages on Ubuntu/Debian:
```bash
sudo apt install debianutils sed make binutils build-essential gcc g++ bash patch \
                 gzip bzip2 perl tar cpio unzip rsync file bc git
```

### Building ZaraOS

1. **Get Buildroot** (recommended version 2025.05 or later):
```bash
git clone https://gitlab.com/buildroot.org/buildroot.git
cd buildroot
```

2. **Clone ZaraOS external tree**:
```bash
git clone <zaraos-repo-url> ../ZaraOS
```

3. **Configure for your Raspberry Pi model**:

For Raspberry Pi 5:
```bash
make BR2_EXTERNAL=../ZaraOS zaraos_pi5_defconfig
```

For other models, use standard Buildroot defconfigs:
```bash
# Pi 4 (64-bit)
make BR2_EXTERNAL=../ZaraOS raspberrypi4_64_defconfig

# Pi 3 (64-bit) 
make BR2_EXTERNAL=../ZaraOS raspberrypi3_64_defconfig

# Pi Zero 2 W (64-bit)
make BR2_EXTERNAL=../ZaraOS raspberrypizero2w_64_defconfig
```

4. **Customize configuration** (optional):
```bash
make menuconfig
```

5. **Build the system**:
```bash
make
```

This will take some time (consider getting yourself a coffee). The build process downloads, compiles, and assembles all components.

## Build Results

After successful build, you'll find these files in `output/images/`:

```
output/images/
├── bcm27*.dtb              # Device tree files for your Pi model
├── boot.vfat               # Boot partition image  
├── rootfs.ext4             # Root filesystem image
├── rpi-firmware/           # Raspberry Pi firmware files
│   ├── config.txt         # Boot configuration
│   ├── cmdline.txt        # Kernel command line
│   ├── start*.elf         # GPU firmware
│   └── fixup*.dat         # Memory split configuration
├── sdcard.img             # Complete SD card image (flash this!)
├── Image                  # 64-bit kernel (or zImage for 32-bit)
└── genimage.cfg           # Generated image layout config
```

## Flashing to SD Card

**Warning**: This will erase all data on the target device!

```bash
# Find your SD card device (e.g., /dev/sdX, /dev/mmcblkX)
lsblk

# Flash the complete image
sudo dd if=output/images/sdcard.img of=/dev/sdX bs=4M status=progress
sudo sync
```

For faster flashing with verification, use `bmaptool` if available:
```bash
bmaptool copy output/images/sdcard.img /dev/sdX
```

## Boot Process

1. Insert the SD card into your Raspberry Pi
2. Connect power (and optionally HDMI, keyboard)
3. The system will boot and present:
   - **Serial console** on GPIO UART pins (115200 baud)
   - **HDMI console** with login prompt
   - **SSH access** if networking is configured

Default login: `root` (no password)

## Configuration Files

### Boot Configurations (`boot-configs/`)
- `config_5.txt` - Raspberry Pi 5 specific settings
- `config_4*.txt` - Pi 4 configurations (32/64-bit variants)
- `config_3*.txt` - Pi 3 configurations
- `config_*w*.txt` - Pi Zero configurations

### Command Line (`cmdlines/`)
- `cmdline.txt` - Standard kernel parameters
- `cmdline_5.txt` - Pi 5 specific parameters (different UART)

### Build Configurations (`configs/`)
- `zaraos_pi5_defconfig` - Complete Pi 5 configuration

## Customization

### Adding Packages
1. Enable packages in `make menuconfig`
2. Or add to your defconfig file

### Custom Files
- Add files to rootfs using `BR2_ROOTFS_OVERLAY`
- Modify boot files using `BR2_PACKAGE_RPI_FIRMWARE_CONFIG_FILE`

### Post-Build Scripts
- `scripts/post-build.sh` - Modifies target filesystem
- `scripts/post-image.sh` - Generates final images

## Development Workflow

```bash
# Make configuration changes
make menuconfig

# Save your configuration
make savedefconfig
cp defconfig configs/zaraos_custom_defconfig

# Clean and rebuild specific packages
make <package>-rebuild

# Clean everything for fresh build
make clean
```

## Troubleshooting

### Build Issues
- Ensure all dependencies installed
- Check available disk space (builds need several GB)
- Review `output/build/build-time.log` for timing info

### Boot Issues  
- Verify SD card integrity
- Check power supply (Pi 4/5 need 3A+ supply)
- Enable UART debug in config.txt
- Connect serial console to see boot messages

### Image Size
- Default rootfs is 120MB (configurable)
- Adjust `BR2_TARGET_ROOTFS_EXT2_SIZE` for larger images
- Use `du -sh output/target/` to check rootfs size

## Contributing

1. Make changes in appropriate subdirectories
2. Test builds on target hardware  
3. Update documentation
4. Submit pull requests

## License

ZaraOS inherits licensing from Buildroot (GPL-2.0) and individual package licenses. See Buildroot documentation for complete license information.

---

**Project**: Humanoid Robots MQP  
**Target Hardware**: Raspberry Pi (ML workloads)  
**Base**: Buildroot 2025.05+