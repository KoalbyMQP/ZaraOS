.PHONY: build clean test deps assets build-pi

BINARY_NAME=firecracker-all
BUILD_DIR=./bin
ASSETS_DIR=./internal/assets

build: assets
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/firecracker-all

build-pi: assets
	@echo "Building $(BINARY_NAME) for Raspberry Pi..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-arm64 ./cmd/firecracker-all

assets:
	@echo "Downloading assets..."
	@mkdir -p $(ASSETS_DIR)
	@echo "Downloading firecracker binary..."
	@if [ ! -f $(ASSETS_DIR)/firecracker ]; then \
		curl -L -o /tmp/firecracker.tgz https://github.com/firecracker-microvm/firecracker/releases/download/v1.4.1/firecracker-v1.4.1-x86_64.tgz && \
		tar -xzf /tmp/firecracker.tgz -C /tmp && \
		cp /tmp/release-v1.4.1-x86_64/firecracker-v1.4.1-x86_64 $(ASSETS_DIR)/firecracker && \
		rm -rf /tmp/firecracker.tgz /tmp/release-v1.4.1-x86_64; \
	fi
	@echo "Downloading firecracker-containerd shim..."
	@if [ ! -f $(ASSETS_DIR)/containerd-shim-aws-firecracker ]; then \
		curl -L -o /tmp/firecracker-containerd.tgz https://github.com/firecracker-microvm/firecracker-containerd/releases/download/v1.7.0/firecracker-containerd-1.7.0-linux-amd64.tar.gz && \
		tar -xzf /tmp/firecracker-containerd.tgz -C /tmp && \
		cp /tmp/bin/containerd-shim-aws-firecracker $(ASSETS_DIR)/ && \
		rm -rf /tmp/firecracker-containerd.tgz /tmp/bin; \
	fi

assets-pi:
	@echo "Downloading ARM64 assets for Pi..."
	@mkdir -p $(ASSETS_DIR)
	@echo "Downloading firecracker binary for ARM64..."
	@if [ ! -f $(ASSETS_DIR)/firecracker ]; then \
		curl -L -o /tmp/firecracker.tgz https://github.com/firecracker-microvm/firecracker/releases/download/v1.4.1/firecracker-v1.4.1-aarch64.tgz && \
		tar -xzf /tmp/firecracker.tgz -C /tmp && \
		cp /tmp/release-v1.4.1-aarch64/firecracker-v1.4.1-aarch64 $(ASSETS_DIR)/firecracker && \
		rm -rf /tmp/firecracker.tgz /tmp/release-v1.4.1-aarch64; \
	fi
	@echo "Downloading firecracker-containerd shim for ARM64..."
	@if [ ! -f $(ASSETS_DIR)/containerd-shim-aws-firecracker ]; then \
		curl -L -o /tmp/firecracker-containerd.tgz https://github.com/firecracker-microvm/firecracker-containerd/releases/download/v1.7.0/firecracker-containerd-1.7.0-linux-arm64.tar.gz && \
		tar -xzf /tmp/firecracker-containerd.tgz -C /tmp && \
		cp /tmp/bin/containerd-shim-aws-firecracker $(ASSETS_DIR)/ && \
		rm -rf /tmp/firecracker-containerd.tgz /tmp/bin; \
	fi

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(ASSETS_DIR)/firecracker $(ASSETS_DIR)/containerd-shim-aws-firecracker

deps:
	go mod download
	go mod tidy

test: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) test

server: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) server

vm-create: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm create test-vm alpine:latest

vm-destroy: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm destroy test-vm

vm-list: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm list

format:
	go fmt ./...
