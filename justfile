# justfile

RED := '\033[0;31m'
GREEN := '\033[0;32m'
YELLOW := '\033[1;33m'
BLUE := '\033[0;34m'
NC := '\033[0m'

default:
    @just --list

# Build ZaraOS
#TODO: 
# - verify support with booting on pi!
# - add support for other pis
# - make commands more rich
# FIXME:
# - this should be done in a container for even more reliability (this *WILL* be a problem)
# - make the build command multithreaded currentsetup is hardcoded

# Build ZaraOS
build:
    #!/usr/bin/env bash
    set -e
    
    echo -e "{{BLUE}}Building ZaraOS in clean buildroot environment{{NC}}"
    
    if ! podman image exists localhost/zaraos-builder:latest; then
        echo -e "{{YELLOW}}Building clean buildroot container...{{NC}}"
        just container-build-clean
    fi
    
    sudo rm -rf ZaraOS/output ZaraOS/dl 2>/dev/null || rm -rf ZaraOS/output ZaraOS/dl
    mkdir -p ZaraOS/output ZaraOS/dl
    
    podman run --rm -it \
        --userns=keep-id \
        -v "$(pwd):/workspace:Z" \
        localhost/zaraos-builder:latest

container-build-clean:
    echo -e "{{BLUE}}Building clean buildroot container{{NC}}"
    podman build -t zaraos-builder -f infra/container/builder/Containerfile infra/container/builder/

# Alternative: build without cleaning (for debugging)
build-dirty:
    #!/usr/bin/env bash
    set -e
    
    echo -e "{{YELLOW}}Building ZaraOS without cleaning output{{NC}}"
    
    if ! podman image exists localhost/zaraos-builder:latest; then
        echo -e "{{YELLOW}}Building clean buildroot container...{{NC}}"
        just container-build-clean
    fi
    
    mkdir -p ZaraOS/output ZaraOS/dl
    
    podman run --rm -it \
        --userns=keep-id \
        -v "$(pwd):/workspace:Z" \
        localhost/zaraos-builder:latest

clean:
    #!/usr/bin/env bash
    echo -e "{{YELLOW}}Cleaning build artifacts{{NC}}"
    if [ -d "ZaraOS/output" ] || [ -d "ZaraOS/dl" ]; then
        sudo rm -rf ZaraOS/output ZaraOS/dl 2>/dev/null || rm -rf ZaraOS/output ZaraOS/dl
        echo -e "{{GREEN}}Cleaned{{NC}}"
    else
        echo "Nothing to clean"
    fi

# Debug: check what containers exist
list-containers:
    echo -e "{{BLUE}}Available container images:{{NC}}"
    podman images | grep -E "(zaraos|buildroot)" || echo "No ZaraOS containers found"

clean-containers:
    echo -e "{{YELLOW}}Removing ZaraOS containers{{NC}}"
    podman rmi zaraos-builder 2>/dev/null || echo "zaraos-builder not found"
    podman rmi localhost/zaraos-builder:latest 2>/dev/null || echo "localhost/zaraos-builder:latest not found"