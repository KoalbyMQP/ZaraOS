# justfile

RED := '\033[0;31m'
GREEN := '\033[0;32m'
YELLOW := '\033[1;33m'
BLUE := '\033[0;34m'
NC := '\033[0m'

default:
    @just --list

# Build ZaraOS for Pi 5 TODO: verify support with booting on pi!
build:
    #!/usr/bin/env bash
    set -e
    
    CONFIG_PATH="ZaraOS/external/configs/zaraos_pi5_defconfig"
    OUTPUT_DIR="$(pwd)/ZaraOS/output"
    DL_DIR="$(pwd)/ZaraOS/dl"

    # FIXME: Make this a flag to the build command!
    export MAKEFLAGS="-j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 8)"

    echo -e "{{BLUE}}Building ZaraOS{{NC}}"
    
    if [ ! -f "$CONFIG_PATH" ]; then
        echo -e "{{RED}}Config not found: $CONFIG_PATH{{NC}}"
        exit 1
    fi
    
    # Clean PATH to avoid spaces/tabs
    export PATH="/usr/bin:/bin:/usr/sbin:/sbin:$(echo "$PATH" | tr -d ' \t\n')"
    
    # Create directories
    mkdir -p "$OUTPUT_DIR" "$DL_DIR"
    
    export BR2_EXTERNAL="$(pwd)/ZaraOS/external"
    MAKE_FLAGS="HOSTCFLAGS=-Wno-format-security HOSTCXXFLAGS=-Wno-format-security O=$OUTPUT_DIR BR2_DL_DIR=$DL_DIR"
    
    echo -e "{{YELLOW}}Configuring{{NC}}"
    make -C ZaraOS/buildroot $MAKE_FLAGS zaraos_pi5_defconfig
    
    echo -e "{{YELLOW}}Building{{NC}}"
    make -C ZaraOS/buildroot $MAKE_FLAGS
    
    echo -e "{{GREEN}}Build complete{{NC}}"
    echo "Images: $OUTPUT_DIR/images/"

clean:
    #!/usr/bin/env bash
    echo -e "{{YELLOW}}Cleaning build artifacts{{NC}}"
    if [ -d "ZaraOS/output" ] || [ -d "ZaraOS/dl" ]; then
        rm -rf ZaraOS/output ZaraOS/dl
        echo -e "{{GREEN}}Cleaned{{NC}}"
    else
        echo "Nothing to clean"
    fi

status:
    #!/usr/bin/env bash
    echo -e "{{BLUE}}ZaraOS Status{{NC}}"
    
    if [ -L "ZaraOS/buildroot" ]; then
        echo -e "{{GREEN}}Buildroot linked{{NC}}"
    else
        echo -e "{{RED}}Buildroot not linked{{NC}}"
    fi
    
    if [ -f "ZaraOS/external/configs/zaraos_pi5_defconfig" ]; then
        echo -e "{{GREEN}}Pi 5 config present{{NC}}"
    else
        echo -e "{{RED}}Pi 5 config missing{{NC}}"
    fi
    
    if [ -d "ZaraOS/output/images" ]; then
        echo -e "{{GREEN}}Build artifacts present{{NC}}"
    else
        echo -e "{{YELLOW}}No build artifacts{{NC}}"
    fi