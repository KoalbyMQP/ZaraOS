#!/bin/sh
# ===================================================================
# ZaraOS Startup Script
# ===================================================================
# This script runs automatically when a user logs in via auto-login.
# It executes the ZaraOS demo script to show system capabilities.
# ===================================================================

# Only run on interactive login, not for non-interactive shells
case $- in
    *i*) ;;
    *) return ;;
esac

# Only run once per boot (check if already running)
if [ -f /tmp/zaraos-demo-running ]; then
    echo "ZaraOS demo already running in background"
    echo "Type 'zaraos-demo' to run it again"
    return
fi

# Check if this is the first login after boot
if [ ! -f /tmp/zaraos-first-boot ]; then
    echo "Starting ZaraOS demonstration script..."
    echo ""

    # Mark that we've done first boot
    touch /tmp/zaraos-first-boot

    # Run the demo script
    if [ -x /usr/local/bin/demo.py ]; then
        python3 /usr/local/bin/demo.py
    else
        echo "ZaraOS demo script not found at /usr/local/bin/demo.py"
    fi

    echo ""
    echo "Demo completed. You now have a shell."
    echo "Type 'zaraos-demo' to run the demo again."
fi

# Create alias for easy access
alias zaraos-demo='python3 /usr/local/bin/demo.py'
