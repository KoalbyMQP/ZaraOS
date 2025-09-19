.PHONY: build clean test install deps

BINARY_NAME=firecracker-manager
BUILD_DIR=./bin

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/manager

build-pi:
	@echo "Building $(BINARY_NAME) for Raspberry Pi..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-arm64 ./cmd/manager

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

test:
	go test -v ./...

deps:
	go mod download
	go mod tidy

install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run-test: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -action=test

run-create: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -action=create -id=test-vm

run-destroy: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -action=destroy -id=test-vm

lint:
	golangci-lint run

format:
	go fmt ./...
