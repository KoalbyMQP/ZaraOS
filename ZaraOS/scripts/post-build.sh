#!/bin/sh
# ===================================================================
# ZaraOS Post-Build Script
# ===================================================================
# This script runs after the root filesystem is built but before
# creating the final filesystem images. It customizes the target
# filesystem for ZaraOS requirements.
#
# Documentation:
# - Buildroot Post-Build: https://buildroot.org/downloads/manual/manual.html#rootfs-custom
# - System Configuration: https://buildroot.org/downloads/manual/manual.html#customize-rootfs
#
# Environment:
# - TARGET_DIR: Path to the target root filesystem directory
# - Called from Buildroot's main Makefile during build process
# ===================================================================

set -u  # Exit on undefined variables
set -e  # Exit on any error

# ┌─────────────────────────────────────────────────────────────────┐
# │ CONSOLE CONFIGURATION                                           │
# └─────────────────────────────────────────────────────────────────┘

# Add HDMI console support alongside serial console
# This ensures users can access the system via monitor+keyboard
# in addition to the serial UART console

if [ -e ${TARGET_DIR}/etc/inittab ]; then
    # For SysV init systems (BusyBox default)
    echo "Configuring HDMI console in /etc/inittab"
    
    # Check if tty1 console already exists
    if ! grep -qE '^tty1::' ${TARGET_DIR}/etc/inittab; then
        # Add tty1 console entry after the GENERIC_SERIAL line
        sed -i '/GENERIC_SERIAL/a\
tty1::respawn:/sbin/getty -L  tty1 0 vt100 # HDMI console' ${TARGET_DIR}/etc/inittab
        echo "Added HDMI console (tty1) to inittab"
    else
        echo "HDMI console already configured in inittab"
    fi

elif [ -d ${TARGET_DIR}/etc/systemd ]; then
    # For systemd-based systems
    echo "Configuring HDMI console for systemd"
    
    # Create systemd service directory
    mkdir -p "${TARGET_DIR}/etc/systemd/system/getty.target.wants"
    
    # Enable getty service on tty1 (HDMI console)
    ln -sf /lib/systemd/system/getty@.service \
       "${TARGET_DIR}/etc/systemd/system/getty.target.wants/getty@tty1.service"
    echo "Enabled HDMI console service for systemd"
    
else
    echo "Warning: Unknown init system, console configuration may be incomplete"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ ZARAOS SPECIFIC CUSTOMIZATIONS                                  │
# └─────────────────────────────────────────────────────────────────┘

# Create ZaraOS identification file
echo "Creating ZaraOS system identification..."
cat > ${TARGET_DIR}/etc/zaraos-release << EOF
ZaraOS for Humanoid Robots MQP
Built on: $(date)
Target: Raspberry Pi 5
Buildroot: $(cat ${BR2_VERSION_FULL:-"unknown"} 2>/dev/null || echo "unknown")
Architecture: aarch64
EOF

# Set hostname to zaraos
echo "zaraos" > ${TARGET_DIR}/etc/hostname
echo "Set hostname to 'zaraos'"

# Configure welcome message
if [ -d ${TARGET_DIR}/etc ]; then
    cat > ${TARGET_DIR}/etc/motd << EOF

 ______                  _____ _____ 
|___  /                 |  _  /  ___|
   / / __ _ _ __ __ _    | | | \ \`--. 
  / / / _\` | '__/ _\` |   | | | |\`--. \\
 / /_| (_| | | | (_| |   \ \_/ /\__/ /
/_____\__,_|_|  \__,_|    \___/\____/ 

Raspberry Pi OS for Humanoid Robots MQP
Type 'help' for available commands.

Serial Console: Available on GPIO UART (115200 baud)
HDMI Console:   Connect monitor and keyboard

EOF
    echo "Created welcome message"
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ NETWORK CONFIGURATION                                           │
# └─────────────────────────────────────────────────────────────────┘

# Ensure network interfaces are properly configured
if [ -d ${TARGET_DIR}/etc/network ]; then
    # Create basic network configuration if it doesn't exist
    if [ ! -f ${TARGET_DIR}/etc/network/interfaces ]; then
        cat > ${TARGET_DIR}/etc/network/interfaces << EOF
# Network interfaces configuration for ZaraOS
auto lo
iface lo inet loopback

# Ethernet interface with DHCP (configured in defconfig)
auto eth0
iface eth0 inet dhcp

# Wireless interface (if available and configured)
# auto wlan0  
# iface wlan0 inet dhcp
EOF
        echo "Created basic network interfaces configuration"
    fi
fi

# ┌─────────────────────────────────────────────────────────────────┐
# │ DEVELOPMENT AND DEBUG HELPERS                                   │
# └─────────────────────────────────────────────────────────────────┘

# Create useful aliases for development
if [ -d ${TARGET_DIR}/etc ]; then
    cat > ${TARGET_DIR}/etc/profile.d/zaraos-aliases.sh << 'EOF'
# ZaraOS development aliases
alias ll='ls -la'
alias temp='vcgencmd measure_temp'
alias volts='vcgencmd measure_volts'
alias freq='vcgencmd measure_clock arm'
alias throttle='vcgencmd get_throttled'
alias gpio='vcgencmd get_config'
EOF
    chmod +x ${TARGET_DIR}/etc/profile.d/zaraos-aliases.sh
    echo "Created ZaraOS development aliases"
fi

# Basic Heartbeat to indicate system is alive
# FIXME: This should probably be taken out for production
cat > ${TARGET_DIR}/etc/init.d/S99boot_signal << 'EOF'
#!/bin/sh
case "$1" in
    start)
        echo "ZaraOS heartbeat starting..."
        
        # Initial "I'm alive" sequence
        # Pattern: SHORT-short-LONG-LONG-short-SHORT (like a heartbeat)
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.05
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.3
        
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.3
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.3
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.05
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        
        echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 0.1
        echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
        sleep 3.0
        
        # Then repeat every 10 seconds to show "still alive"
        while true; do
            echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
            sleep 0.1
            echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
            sleep 0.1
            echo 1 > /sys/class/leds/ACT/brightness 2>/dev/null || true
            sleep 0.1
            echo 0 > /sys/class/leds/ACT/brightness 2>/dev/null || true
            sleep 10.0
        done &
        ;;
esac
EOF
chmod +x ${TARGET_DIR}/etc/init.d/S99boot_signal

# ┌─────────────────────────────────────────────────────────────────┐
# │ SECURITY AND OPTIMIZATION                                       │
# └─────────────────────────────────────────────────────────────────┘

# Remove unnecessary files to reduce filesystem size
echo "Optimizing filesystem size"

# Remove documentation that's not essential for embedded system
if [ -d ${TARGET_DIR}/usr/share/doc ]; then
    rm -rf ${TARGET_DIR}/usr/share/doc/*
    echo "Cleaned documentation files"
fi

# Remove man pages (uncomment if space is critical)
# if [ -d ${TARGET_DIR}/usr/share/man ]; then
#     rm -rf ${TARGET_DIR}/usr/share/man/*
#     echo "Cleaned man pages"
# fi

# Set appropriate permissions for security
echo "Setting file permissions"

# Ensure root directories have correct permissions
chmod 755 ${TARGET_DIR}/etc
chmod 755 ${TARGET_DIR}/var
chmod 1777 ${TARGET_DIR}/tmp 2>/dev/null || true  # sticky bit for tmp

echo "Set security permissions"

# ┌─────────────────────────────────────────────────────────────────┐
# │ COMPLETION                                                      │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "ZaraOS post-build script completed successfully"
echo "  - Console configuration: HDMI + Serial"
echo "  - System identification: /etc/zaraos-release"
echo "  - Hostname: zaraos"
echo "  - Network: DHCP on eth0"
echo "  - Development aliases: /etc/profile.d/zaraos-aliases.sh"
echo "  - Filesystem optimization: Complete"
echo "═══════════════════════════════════════════════════════════════"
echo ""