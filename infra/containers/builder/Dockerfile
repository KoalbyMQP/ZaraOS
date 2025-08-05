FROM docker.io/library/ubuntu:22.04

# Install ZaraOS build dependencies + GitHub Actions runner dependencies
RUN apt-get update && apt-get install -y \
    build-essential gcc g++ gfortran \
    libncurses5-dev bc bison flex gperf \
    unzip gzip bzip2 xz-utils cpio \
    wget rsync git subversion mercurial \
    texinfo help2man python3 python3-dev \
    file debianutils sed gawk \
    libssl-dev pkg-config \
    # GitHub Actions runner dependencies
    curl jq sudo ca-certificates \
    libicu70 liblttng-ust1 libkrb5-3 zlib1g \
    libssl3 libunwind8 && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Install GitHub Actions Runner
ARG RUNNER_VERSION="2.311.0"
RUN mkdir -p /opt/actions-runner && cd /opt/actions-runner \
    && curl -o actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz -L https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz \
    && tar xzf ./actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz \
    && rm actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz \
    && ./bin/installdependencies.sh

COPY ZaraOS/buildroot /opt/buildroot

# Create runner user with sudo access
RUN useradd -m -s /bin/bash runner && \
    usermod -aG sudo runner && \
    echo 'runner ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

# Set up runner ownership and make build tools available to runner
RUN chown -R runner:runner /opt/actions-runner

# Copy and set up build script
COPY infra/containers/builder/build-zaraos.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/build-zaraos.sh

# Make build-zaraos available as a simple command
RUN ln -s /usr/local/bin/build-zaraos.sh /usr/local/bin/build-zaraos

USER runner
WORKDIR /home/runner

# GitHub Actions runner entrypoint
CMD ["/opt/actions-runner/bin/runsvc.sh"]