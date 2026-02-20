.PHONY: build run clean docker-build docker-run docker-stop

BINARY    = cortex
IMAGE     = cortex:latest
CONTAINER = cortex

build:
	go build -o bin/$(BINARY) ./cmd/server

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin/

# Cross-compile for Raspberry Pi (arm64)
build-pi:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/$(BINARY)-arm64 ./cmd/server

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm --name $(CONTAINER) -p 8080:8080 $(IMAGE)

docker-stop:
	docker stop $(CONTAINER)
