CORTEX_VERSION = local
CORTEX_SITE = $(BR2_EXTERNAL_ZaraOS_PATH)/../cortex
CORTEX_SITE_METHOD = local
CORTEX_BUILD_TARGETS = cmd

define CORTEX_INSTALL_TARGET_CMDS
    $(INSTALL) -D -m 0755 $(@D)/bin/cmd $(TARGET_DIR)/usr/bin/cortex
endef

$(eval $(golang-package))
