# ZaraOS Development Environment Dockerfile for DevPod
FROM mcr.microsoft.com/devcontainers/base:ubuntu-22.04

# Install development tools
RUN apt-get update && apt-get install -y \
    # Essential development tools
    git curl wget vim \
    build-essential \
    # Shell and utilities
    zsh tmux htop tree \
    # Archive tools
    unzip zip \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/install.sh | sh

# Install just (build tool)
RUN curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

# Setup zsh as default shell for vscode user
RUN chsh -s /usr/bin/zsh vscode

USER vscode

# Default command
CMD ["/usr/bin/zsh"]
