FROM myoung34/github-runner:latest

# Switch to root to install packages
USER root

# Install ZaraOS build dependencies
RUN apt-get update && apt-get install -y \
    build-essential gcc g++ gfortran \
    libncurses5-dev bc bison flex gperf \
    unzip gzip bzip2 xz-utils cpio \
    wget rsync git subversion mercurial \
    texinfo help2man python3 python3-dev \
    file debianutils sed gawk \
    libssl-dev pkg-config && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY ZaraOS/buildroot /opt/buildroot

# Copy and set up build script
COPY infra/containers/builder/build-zaraos.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/build-zaraos.sh

# Make build-zaraos available as a simple command
RUN ln -s /usr/local/bin/build-zaraos.sh /usr/local/bin/build-zaraos

# Switch back to runner user (inherited from base image)
USER runner