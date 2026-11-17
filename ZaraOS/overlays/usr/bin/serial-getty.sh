#!/bin/sh
# ===================================================================
# serial-getty.sh - autologin getty on a serial port, if it exists
# ===================================================================
# Usage (from /etc/inittab):  ::respawn:/usr/bin/serial-getty.sh TTY [BAUD]
#
# The Pi 5 debug UART is ttyAMA10, QEMU virt's is ttyAMA0, and each
# platform lacks the other one. BusyBox init does NOT skip inittab
# entries for missing ttys — it fails to open the device and respawns
# the entry every second, flooding the console with
# "can't open /dev/ttyAMAxx". So the inittab entries leave the tty field
# empty and this wrapper checks for the device itself; if it's missing
# it parks forever so init has nothing to respawn.
# ===================================================================

TTY="$1"
BAUD="${2:-115200}"

if [ -z "$TTY" ] || [ ! -c "/dev/$TTY" ]; then
    exec sleep 2147483647
fi

exec /sbin/getty -n -L -l /usr/bin/autologin.sh "$TTY" "$BAUD" vt100
