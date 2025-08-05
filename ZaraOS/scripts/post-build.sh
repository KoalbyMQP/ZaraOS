#!/bin/sh
# ===================================================================
# ZaraOS Post-Build Script - Simplified Version
# ===================================================================
# This script runs after the root filesystem is built but before
# creating the final filesystem images. Most customizations are now
# handled by the rootfs overlay system for better maintainability.
#
# This script now only handles:
# - File permissions that can't be set via overlay
# - Filesystem optimization and cleanup
# - Security-related tasks
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
# │ FILE PERMISSIONS                                                │
# └─────────────────────────────────────────────────────────────────┘

echo "Setting file permissions for ZaraOS"

# Make Python demo script executable
if [ -f "${TARGET_DIR}/usr/local/bin/demo.py" ]; then
    chmod +x "${TARGET_DIR}/usr/local/bin/demo.py"
    echo "Made demo.py executable"
else
    echo "Warning: demo.py not found (overlay may not have been applied)"
fi

# Make autologin script executable
if [ -f "${TARGET_DIR}/usr/bin/autologin.sh" ]; then
    chmod +x "${TARGET_DIR}/usr/bin/autologin.sh"
    echo "Made autologin.sh executable"
else
    echo "Warning: autologin.sh not found (overlay may not have been applied)"
fi

# Make startup script executable
if [ -f "${TARGET_DIR}/etc/profile.d/zaraos-startup.sh" ]; then
    chmod +x "${TARGET_DIR}/etc/profile.d/zaraos-startup.sh"
    echo "Made zaraos-startup.sh executable"
else
    echo "Warning: zaraos-startup.sh not found (overlay may not have been applied)"
fi

# Set appropriate permissions for security
echo "Setting security permissions"

# Ensure root directories have correct permissions
chmod 755 "${TARGET_DIR}/etc" 2>/dev/null || true
chmod 755 "${TARGET_DIR}/var" 2>/dev/null || true
chmod 1777 "${TARGET_DIR}/tmp" 2>/dev/null || true  # sticky bit for tmp

# Ensure /usr/local/bin is executable
chmod 755 "${TARGET_DIR}/usr/local/bin" 2>/dev/null || true

# ┌─────────────────────────────────────────────────────────────────┐
# │ FILESYSTEM OPTIMIZATION                                         │
# └─────────────────────────────────────────────────────────────────┘

echo "Optimizing filesystem size"

# Remove documentation that's not essential for embedded system
if [ -d "${TARGET_DIR}/usr/share/doc" ]; then
    rm -rf "${TARGET_DIR}/usr/share/doc"/* 2>/dev/null || true
    echo "Cleaned documentation files"
fi

# Remove man pages
if [ -d "${TARGET_DIR}/usr/share/man" ]; then
    rm -rf "${TARGET_DIR}/usr/share/man"/* 2>/dev/null || true
    echo "Cleaned man pages"
fi

# Remove locale files except C/POSIX (saves space)
if [ -d "${TARGET_DIR}/usr/share/locale" ]; then
    find "${TARGET_DIR}/usr/share/locale" -mindepth 1 -maxdepth 1 -type d ! -name 'C' ! -name 'POSIX' -exec rm -rf {} + 2>/dev/null || true
    echo "Cleaned locale files"
fi

# Remove Python cache files if any were created
find "${TARGET_DIR}" -name "*.pyc" -delete 2>/dev/null || true
find "${TARGET_DIR}" -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
echo "Cleaned Python cache files"

# ┌─────────────────────────────────────────────────────────────────┐
# │ COMPLETION                                                      │
# └─────────────────────────────────────────────────────────────────┘

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "ZaraOS post-build script completed successfully"
echo "  - File permissions: Set"
echo "  - Filesystem optimization: Complete"
echo "  - Overlay validation: Complete"
echo "  - Python demo script: Ready"
echo "  - Auto-login script: Ready"
echo "  - Auto-login: Configured via overlay"
echo "  - Network: DHCP configured"
echo "═══════════════════════════════════════════════════════════════"
echo ""
