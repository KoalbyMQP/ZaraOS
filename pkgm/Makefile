.PHONY: build clean test deps

BINARY_NAME=firecracker-all
BUILD_DIR=./bin
ASSETS_DIR=./internal/assets

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/manager

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
	$(BUILD_DIR)/$(BINARY_NAME) -action=server -port=8080

vm-create: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm create test-vm alpine:latest

vm-destroy: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm destroy test-vm

vm-list: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) vm list

format:
	go fmt ./...
