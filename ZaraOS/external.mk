# ===================================================================
# ZaraOS External Makefile
# ===================================================================
# This file contains custom build logic and make targets for the
# ZaraOS external tree. It's included by Buildroot's main Makefile.
#
# Documentation:
# - Buildroot External Trees: https://buildroot.org/downloads/manual/manual.html#outside-br-custom
# - External Tree Makefiles: https://buildroot.org/downloads/manual/manual.html#_the_external_mk_file
# - Makefile Syntax: https://buildroot.org/downloads/manual/manual.html#writing-rules-mk
#
# This file can define:
# - Custom make targets
# - Additional include paths
# - Global build customizations
# ===================================================================

# Add custom make targets and build logic here
# Example:
# include $(BR2_EXTERNAL_ZaraOS_PATH)/package/*/*.mk