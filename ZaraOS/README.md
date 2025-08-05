# ZaraOS - Raspberry Pi Custom Linux Distribution

ZaraOS is a specialized Linux distribution built on Buildroot, specifically designed for the Humanoid Robots MQP project. It provides a minimal, optimized operating system tailored for Raspberry Pi hardware with ML workload capabilities.

## Documentation Links
- **Buildroot Manual**: https://buildroot.org/downloads/manual/manual.html
- **Raspberry Pi Documentation**: https://www.raspberrypi.com/documentation/
- **External Trees Guide**: https://buildroot.org/downloads/manual/manual.html#outside-br-custom
- **Buildroot Training**: https://bootlin.com/doc/training/buildroot/

## Supported Raspberry Pi Models

ZaraOS supports the following Raspberry Pi models:
- Raspberry Pi 5 (primary target)

## Quick Start

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
