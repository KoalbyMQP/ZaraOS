FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC
ENV FORCE_UNSAFE_CONFIGURE=1
ENV CONFIG_SHELL=/bin/bash

RUN apt-get update && apt-get install -y \
    curl wget git ca-certificates \
    build-essential gcc g++ gfortran \
    libncurses5-dev bc bison flex gperf \
    unzip gzip bzip2 xz-utils cpio \
    rsync texinfo help2man \
    python3 python3-dev python3-setuptools \
    file debianutils sed gawk \
    libssl-dev pkg-config \
    make cmake automake autoconf libtool \
    openssh-client sudo \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
    gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
    tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1000 jenkins && \
    useradd -u 1000 -g jenkins -m -s /bin/bash jenkins && \
    echo 'jenkins ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

COPY ZaraOS/buildroot /opt/buildroot

RUN chown -R root:root /opt/buildroot && \
    chmod -R 755 /opt/buildroot

COPY infra/containers/builder/build-zaraos.sh /usr/local/bin/build-zaraos.sh
RUN chmod +x /usr/local/bin/build-zaraos.sh && \
    ln -s /usr/local/bin/build-zaraos.sh /usr/local/bin/build-zaraos

WORKDIR /workspace
USER root

CMD ["/bin/bash"]
