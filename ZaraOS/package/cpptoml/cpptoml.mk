################################################################################
#
# cpptoml
#
################################################################################

# Pinned to the version iceoryx 2.0.x fetches itself (DOWNLOAD_TOML_LIB).
CPPTOML_VERSION = 0.1.1
CPPTOML_SITE = $(call github,skystrife,cpptoml,v$(CPPTOML_VERSION))
CPPTOML_LICENSE = MIT
CPPTOML_LICENSE_FILES = LICENSE

# Header-only: install headers + CMake config to staging only.
CPPTOML_INSTALL_STAGING = YES
CPPTOML_INSTALL_TARGET = NO

CPPTOML_CONF_OPTS = \
	-DENABLE_LIBCXX=OFF \
	-DCPPTOML_BUILD_EXAMPLES=OFF \
	-DCMAKE_POLICY_VERSION_MINIMUM=3.5

$(eval $(cmake-package))
