#!/bin/sh
# Temporary boot diagnostics: snapshot kernel log + hardware state to
# /data/debug every 2s for ~3 minutes, so it survives a hang/power-pull.

case "$1" in
    start)
        mkdir -p /data/debug
        (
            n=0
            while [ $n -lt 90 ]; do
                {
                    echo "=== uptime";  cat /proc/uptime
                    echo "=== net";     ls /sys/class/net; ip link 2>&1 || ifconfig -a 2>&1
                    echo "=== pci";     ls -l /sys/bus/pci/devices 2>&1
                    echo "=== usb";     lsusb 2>&1; ls /sys/bus/usb/devices 2>&1
                    echo "=== input";   cat /proc/bus/input/devices 2>&1
                    echo "=== leds";    for l in /sys/class/leds/*; do echo "$l: $(cat $l/trigger 2>/dev/null | grep -o '\[[^]]*\]')"; done
                    echo "=== ps";      ps 2>&1
                    echo "=== mem";     head -5 /proc/meminfo
                } > /data/debug/state.txt 2>&1
                dmesg > /data/debug/dmesg.txt 2>&1
                cp /var/log/messages /data/debug/messages.txt 2>/dev/null
                echo "$n $(cut -d' ' -f1 /proc/uptime)" >> /data/debug/heartbeat.txt
                sync
                n=$((n + 1))
                sleep 2
            done
        ) < /dev/null > /dev/null 2>&1 &
        ;;
    stop) ;;
esac
