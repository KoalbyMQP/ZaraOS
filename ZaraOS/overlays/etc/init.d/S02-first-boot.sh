#!/bin/sh
# First-boot: expand the data partition (p4) to fill the rest of the SD card.
# Uses a flag file on /data so this only runs once.
# Requires: fdisk (busybox), resize2fs (e2fsprogs), partprobe or reboot.

DISK=/dev/mmcblk0
DATA_PART=${DISK}p4
FLAG=/data/.first-boot-done

case "$1" in
    start)
        # /data must already be mounted (S01-mounts.sh runs first)
        if [ ! -d /data ]; then
            echo "FIRST-BOOT: /data not mounted, skipping resize"
            exit 0
        fi

        if [ -f "$FLAG" ]; then
            exit 0
        fi

        echo "FIRST-BOOT: expanding data partition to fill SD card..."

        # Get the start sector of p4 from the current partition table
        START=$(fdisk -l "$DISK" | awk '/mmcblk0p4/{print $2}')
        if [ -z "$START" ]; then
            echo "FIRST-BOOT: could not determine p4 start sector, aborting"
            exit 1
        fi

        # Unmount data partition for partition table changes
        umount /var/lib/containerd 2>/dev/null || true
        umount /etc/cni 2>/dev/null || true
        umount /data

        # Delete p4 and recreate from same start to end of disk
        # sfdisk: start,size,type  — empty size = fill remaining
        sfdisk --force "$DISK" -N 4 <<EOF
${START},,L
EOF

        # Inform kernel of partition table change
        partprobe "$DISK" 2>/dev/null || true
        sleep 1

        # Resize the filesystem to fill the new partition
        resize2fs "$DATA_PART"

        # Remount
        mount -t ext4 "$DATA_PART" /data
        mkdir -p /data/containerd /data/cni
        mount --bind /data/containerd /var/lib/containerd
        mount --bind /data/cni /etc/cni

        # Write flag so we never run again
        touch "$FLAG"
        echo "FIRST-BOOT: data partition expanded successfully."
        ;;
    stop)
        ;;
    *)
        echo "Usage: $0 {start|stop}"
        exit 1
        ;;
esac
