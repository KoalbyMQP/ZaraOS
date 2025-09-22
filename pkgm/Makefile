.PHONY: build clean deps format setup-firecracker

BINARY_NAME=pkgmngr
BUILD_DIR=./bin

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/manager

# Download firecracker binary and setup basic resources
setup-firecracker:
	@echo "Setting up firecracker..."
	@sudo mkdir -p /var/lib/firecracker
	@if [ ! -f /usr/local/bin/firecracker ]; then \
		curl -L -o /tmp/firecracker https://github.com/firecracker-microvm/firecracker/releases/download/v1.10.0/firecracker-v1.10.0-x86_64 && \
		sudo cp /tmp/firecracker /usr/local/bin/firecracker && \
		sudo chmod +x /usr/local/bin/firecracker && \
		rm /tmp/firecracker; \
	fi
	@if [ ! -f /var/lib/firecracker/vmlinux.bin ]; then \
		curl -L -o /tmp/vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin && \
		sudo cp /tmp/vmlinux.bin /var/lib/firecracker/vmlinux.bin && \
		rm /tmp/vmlinux.bin; \
	fi

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

deps:
	go mod download
	go mod tidy

test: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -action=test

healthcheck: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -action=healthcheck

format:
	go fmt ./...