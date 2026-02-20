#!/bin/sh
# Mount /boot (FAT32, p1) and /data (EXT4, p4) early so they are available
# for first-boot resize and containerd before S90 starts.

BOOT_DEV=/dev/mmcblk0p1
DATA_DEV=/dev/mmcblk0p4

case "$1" in
    start)
        echo "Mounting /boot..."
        mkdir -p /boot
        mount -t vfat "$BOOT_DEV" /boot || echo "WARNING: failed to mount /boot"

        echo "Mounting /data..."
        mkdir -p /data
        mount -t ext4 "$DATA_DEV" /data || echo "WARNING: failed to mount /data"

        # Ensure containerd storage lives on the data partition
        mkdir -p /data/containerd
        mkdir -p /var/lib/containerd
        mount --bind /data/containerd /var/lib/containerd

        # CNI config on data so it survives updates
        mkdir -p /data/cni
        mkdir -p /etc/cni
        mount --bind /data/cni /etc/cni

        echo "Mounts done."
        ;;
    stop)
        umount /var/lib/containerd 2>/dev/null || true
        umount /etc/cni 2>/dev/null || true
        umount /data 2>/dev/null || true
        umount /boot 2>/dev/null || true
        ;;
    *)
        echo "Usage: $0 {start|stop}"
        exit 1
        ;;
esac
