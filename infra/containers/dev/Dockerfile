# Stage 1: Build Cortex server and CLI binaries.
FROM golang:1.24 AS builder

WORKDIR /src
COPY cortex/ .

RUN go build -o /out/cortex-server ./cmd/main.go && \
    go build -o /out/cortex ./cmd/cli/

# Stage 2: Runtime image based on ROS Humble.
FROM ros:humble

# Install Docker-in-Docker runtime and ROS dev tools.
RUN apt-get update && apt-get install -y \
    docker.io \
    curl \
    wget \
    git \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Cortex currently shells out to `nerdctl`; point that name at Docker so
# the existing code and CLI run unchanged in the dind setup.
RUN ln -sf /usr/bin/docker /usr/local/bin/nerdctl

# Install rosdep and colcon.
RUN apt-get update && apt-get install -y \
    python3-rosdep \
    python3-colcon-common-extensions \
    ros-humble-demo-nodes-cpp \
    ros-humble-rmw-cyclonedds-cpp \
    && apt-get clean && rm -rf /var/lib/apt/lists/* \
    && rosdep init || true

# Install just (build tool).
RUN curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

# Install uv (Python package manager).
RUN curl -LsSf https://astral.sh/uv/install.sh | sh

# Copy Cortex binaries from builder stage.
COPY --from=builder /out/cortex-server /usr/local/bin/cortex-server
COPY --from=builder /out/cortex /usr/local/bin/cortex

# Copy startup script.
COPY infra/containers/dev/start.sh /usr/local/bin/start.sh
RUN chmod +x /usr/local/bin/start.sh

WORKDIR /workspace

EXPOSE 8080

CMD ["/bin/bash"]
