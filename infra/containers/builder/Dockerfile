FROM docker.io/library/ubuntu:22.04

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

RUN useradd -m -s /bin/bash builder
USER builder
WORKDIR /workspace

COPY build-zaraos.sh /usr/local/bin/
USER root
RUN chmod +x /usr/local/bin/build-zaraos.sh
USER builder

ENTRYPOINT ["/usr/local/bin/build-zaraos.sh"]