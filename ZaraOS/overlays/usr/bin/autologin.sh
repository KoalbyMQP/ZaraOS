#!/bin/sh
# ===================================================================
# ZaraOS Auto-Login Script
# ===================================================================
# This script is called by getty to automatically log in as root
# without prompting for username or password. It uses the -f flag
# to force login without password verification.
#
# Called by: getty -l /usr/bin/autologin.sh
# Purpose: Automatically login as root user
# ===================================================================

# Execute login as root user without password prompt
# The 'exec' replaces this script process with login, ensuring
# proper process chain and signal handling
exec /bin/login -f root
