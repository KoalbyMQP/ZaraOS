#!/bin/sh
# shellcheck disable=SC2154  # BR2_EXTERNAL_ZaraOS_PATH and TARGET_DIR injected by Buildroot
set -u
set -e

# ┌─────────────────────────────────────────────────────────────────┐
# │ VERSION INJECTION                                               │
# └─────────────────────────────────────────────────────────────────┘

VERSION_FILE="${BR2_EXTERNAL_ZaraOS_PATH}/VERSION"
if [ ! -f "$VERSION_FILE" ]; then
    echo "ERROR: VERSION file not found at ${VERSION_FILE}"
    exit 1
fi

# shellcheck source=/dev/null
. "$VERSION_FILE"
echo "Injecting versions: BOARD=${BOARD_VERSION} OS=${OS_VERSION}"

cat > "${TARGET_DIR}/etc/zaraos-release" <<EOF
BOARD_VERSION=${BOARD_VERSION}
OS_VERSION=${OS_VERSION}
HOSTNAME=zaraos
TARGET=Raspberry Pi 5
ARCH=aarch64
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Written /etc/zaraos-release"

# ┌─────────────────────────────────────────────────────────────────┐
# │ PERMISSIONS                                                     │
# └─────────────────────────────────────────────────────────────────┘

find "${TARGET_DIR}/etc/init.d" -type f -exec chmod +x {} +
find "${TARGET_DIR}/usr/bin"    -type f -exec chmod +x {} +

# ┌─────────────────────────────────────────────────────────────────┐
# │ SECURITY                                                        │
# └─────────────────────────────────────────────────────────────────┘

chmod 755  "${TARGET_DIR}/etc"          2>/dev/null || true
chmod 755  "${TARGET_DIR}/var"          2>/dev/null || true
chmod 1777 "${TARGET_DIR}/tmp"          2>/dev/null || true
chmod 755  "${TARGET_DIR}/usr/local/bin" 2>/dev/null || true

# ┌─────────────────────────────────────────────────────────────────┐
# │ CLEANUP                                                         │
# └─────────────────────────────────────────────────────────────────┘

rm -rf "${TARGET_DIR}/usr/share/doc"/* 2>/dev/null || true
rm -rf "${TARGET_DIR}/usr/share/man"/* 2>/dev/null || true
find "${TARGET_DIR}/usr/share/locale" -mindepth 1 -maxdepth 1 -type d \
    ! -name 'C' ! -name 'POSIX' -exec rm -rf {} + 2>/dev/null || true
find "${TARGET_DIR}" -name "*.pyc" -delete 2>/dev/null || true
find "${TARGET_DIR}" -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true

echo "post-build complete: board=${BOARD_VERSION} os=${OS_VERSION}"
