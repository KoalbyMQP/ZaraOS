# justfile

RED := '\033[0;31m'
GREEN := '\033[0;32m'
YELLOW := '\033[1;33m'
BLUE := '\033[0;34m'
NC := '\033[0m'

IMAGE_NAME := "zaraos-builder"

default:
    @just --list

# Build using hosted container
build:
    #!/usr/bin/env bash
    # TODO: convert this to run the github action we want to avoid local builds
    echo -e "{{RED}}This is a placeholder for the build step. Please use 'just test-container' to build and test locally.{{NC}}"
    just test-container 

# Build container image locally
build-container:
    echo -e "{{BLUE}}Building container image{{NC}}"
    podman build -t zaraos-builder -f infra/containers/builder/Dockerfile .

# Build and test locally before pushing
test-container:
    just build-container
    rm -rf output 2>/dev/null || true
    mkdir -p output
    podman run --rm -v "$(pwd):/workspace" {{IMAGE_NAME}}

clean:
    #!/usr/bin/env bash
    echo -e "{{YELLOW}}Cleaning output{{NC}}"
    rm -rf output 2>/dev/null || echo "Nothing to clean"

clean-containers:
    echo -e "{{YELLOW}}Removing local containers{{NC}}"
    podman rmi {{IMAGE_NAME}} 2>/dev/null || echo "{{IMAGE_NAME}} not found"
