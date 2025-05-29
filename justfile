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
    
    echo -e "{{BLUE}}Building ZaraOS in container{{NC}}"

    # Check if podman machine is running
    if ! podman machine list 2>/dev/null | grep -q "Currently running"; then
        echo -e "{{YELLOW}}Podman machine not running, starting...{{NC}}"
        if ! podman machine list 2>/dev/null | grep -q "podman-machine-default"; then
            echo -e "{{YELLOW}}Initializing Podman machine...{{NC}}"
            podman machine init
        fi
        podman machine start
    fi
    
    # Ensure container image exists
    if ! podman image exists zaraos-builder; then
        echo -e "{{YELLOW}}Container image not found, building...{{NC}}"
        just container-build
    fi
    
    mkdir -p ZaraOS/output ZaraOS/dl
    
    podman run --rm \
        -v "$(pwd):/workspace:Z" \
        -v "zaraos-nix:/nix:Z" \
        zaraos-builder

clean:
    #!/usr/bin/env bash
    echo -e "{{YELLOW}}Cleaning build artifacts{{NC}}"
    if [ -d "ZaraOS/output" ] || [ -d "ZaraOS/dl" ]; then
        rm -rf ZaraOS/output ZaraOS/dl
        echo -e "{{GREEN}}Cleaned{{NC}}"
    else
        echo "Nothing to clean"
    fi

# Build container image
container-build:
    echo -e "{{BLUE}}Building container image{{NC}}"
    podman build -t zaraos-builder -f infra/container/builder/Containerfile infra/container/builder/

# Build using container
container-run:
    #!/usr/bin/env bash
    echo -e "{{BLUE}}Running container{{NC}}"
    mkdir -p ZaraOS/output ZaraOS/dl
    podman run --rm \
        -v "$(pwd):/workspace:Z" \
        -v "zaraos-nix:/nix:Z" \
        zaraos-builder