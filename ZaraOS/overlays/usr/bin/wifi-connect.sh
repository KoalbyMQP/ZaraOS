#!/bin/sh
SSID="$1"
PASS="$2"

cat > /etc/wpa_supplicant.conf << EOF
ctrl_interface=/var/run/wpa_supplicant
ap_scan=1
network={
    ssid="$SSID"
    psk="$PASS"
}
EOF

killall wpa_supplicant 2>/dev/null || true
rm -f /var/run/wpa_supplicant/wlan0
wpa_supplicant -B -i wlan0 -c /etc/wpa_supplicant.conf
udhcpc -i wlan0
