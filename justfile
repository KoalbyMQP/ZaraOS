# justfile

RED := '\033[0;31m'
GREEN := '\033[0;32m'
YELLOW := '\033[1;33m'
BLUE := '\033[0;34m'
NC := '\033[0m'

IMAGE_NAME := "zaraos-builder"
REGISTRY := "ghcr.io/your-username"

default:
    @just --list

# Build using hosted container
build:
    #!/usr/bin/env bash
    set -e
    echo -e "{{BLUE}}Building ZaraOS using hosted container{{NC}}"
    
    rm -rf output 2>/dev/null || true
    mkdir -p output
    
    podman run --rm \
        -v "$(pwd):/workspace" \
        {{REGISTRY}}/{{IMAGE_NAME}}:latest

# Build container image locally
build-container:
    echo -e "{{BLUE}}Building container image{{NC}}"
    podman build -t zaraos-builder -f infra/containers/builder/Dockerfile .

# Push container to registry
push-container:
    echo -e "{{BLUE}}Pushing container to registry{{NC}}"
    podman tag {{IMAGE_NAME}} {{REGISTRY}}/{{IMAGE_NAME}}:latest
    podman push {{REGISTRY}}/{{IMAGE_NAME}}:latest

# Pull latest container
pull-container:
    podman pull {{REGISTRY}}/{{IMAGE_NAME}}:latest

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
    podman rmi {{REGISTRY}}/{{IMAGE_NAME}}:latest 2>/dev/null || echo "Registry image not found"