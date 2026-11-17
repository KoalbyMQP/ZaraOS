################################################################################
#
# iceoryx
#
################################################################################

# MUST match ros-humble-iceoryx-posh in the app images (Humble: 2.0.5).
# RouDi and clients share memory layouts, so a mismatch breaks SHM.
ICEORYX_VERSION = 2.0.5
ICEORYX_SITE = $(call github,eclipse-iceoryx,iceoryx,v$(ICEORYX_VERSION))
ICEORYX_LICENSE = Apache-2.0
ICEORYX_LICENSE_FILES = LICENSE
ICEORYX_SUBDIR = iceoryx_meta
ICEORYX_INSTALL_STAGING = YES
ICEORYX_DEPENDENCIES = acl cpptoml

# Only hoofs + posh (RouDi). Keep compile-time limits at upstream
# defaults so they match the ROS-built clients.
ICEORYX_CONF_OPTS = \
	-DBINDING_C=OFF \
	-DBUILD_TEST=OFF \
	-DBUILD_DOC=OFF \
	-DEXAMPLES=OFF \
	-DINTROSPECTION=OFF \
	-DDDS_GATEWAY=OFF \
	-DROUDI_ENVIRONMENT=OFF \
	-DCCACHE=OFF \
	-DTOML_CONFIG=ON \
	-DDOWNLOAD_TOML_LIB=OFF

$(eval $(cmake-package))
