#!/bin/sh
# Mount /boot and /data partitions early so they are available
# for first-boot resize and containerd before S90 starts.
#
# Auto-detects platform:
#   Pi 5:  /dev/mmcblk0p1 (boot), /dev/mmcblk0p4 (data)
#   QEMU:  no boot partition, /dev/vdb (data)

case "$1" in
    start)
        # ── Detect platform ──────────────────────────────────
        if [ -b /dev/mmcblk0p1 ]; then
            # Raspberry Pi (SD card)
            BOOT_DEV=/dev/mmcblk0p1
            DATA_DEV=/dev/mmcblk0p4
        elif [ -b /dev/vdb ]; then
            # QEMU virt (virtio disks: vda=rootfs, vdb=data)
            BOOT_DEV=""
            DATA_DEV=/dev/vdb
        else
            echo "WARNING: no known boot/data devices found"
            BOOT_DEV=""
            DATA_DEV=""
        fi

        # ── Mount /boot (Pi only) ────────────────────────────
        if [ -n "$BOOT_DEV" ] && [ -b "$BOOT_DEV" ]; then
            echo "Mounting /boot ($BOOT_DEV)..."
            mkdir -p /boot
            mount -t vfat "$BOOT_DEV" /boot || echo "WARNING: failed to mount /boot"
        fi

        # ── Mount /data (skip if rcS already mounted it) ────
        if mountpoint -q /data 2>/dev/null; then
            echo "/data already mounted (by rcS), skipping."
        elif [ -n "$DATA_DEV" ] && [ -b "$DATA_DEV" ]; then
            # Format on first use if needed
            if ! blkid "$DATA_DEV" 2>/dev/null | grep -q 'TYPE='; then
                echo "First boot: formatting data partition ($DATA_DEV)..."
                mkfs.ext4 -q -L zaraos-data "$DATA_DEV" 2>/dev/null || \
                    echo "WARNING: failed to format $DATA_DEV"
            fi

            echo "Mounting /data ($DATA_DEV)..."
            mkdir -p /data
            mount -t ext4 "$DATA_DEV" /data || echo "WARNING: failed to mount /data"
        else
            echo "No data partition found, using tmpfs /data"
            mkdir -p /data
            mount -t tmpfs tmpfs /data
        fi

        # Ensure containerd storage lives on the data partition
        # (rcS may have already done this — check before re-mounting)
        mkdir -p /data/containerd /var/lib/containerd
        mountpoint -q /var/lib/containerd 2>/dev/null || mount --bind /data/containerd /var/lib/containerd

        # CNI config on data so it survives updates.
        # Copy default CNI config to /data if not already there.
        mkdir -p /data/cni/net.d /etc/cni
        if [ ! -f /data/cni/net.d/10-zaraos.conflist ] && [ -f /etc/cni/net.d/10-zaraos.conflist ]; then
            cp -r /etc/cni/net.d/* /data/cni/net.d/ 2>/dev/null || true
        fi
        mountpoint -q /etc/cni 2>/dev/null || mount --bind /data/cni /etc/cni

        # Persistent config directory for setup wizard and robot settings
        mkdir -p /data/config

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
