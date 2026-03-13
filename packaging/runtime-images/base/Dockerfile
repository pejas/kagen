# syntax=docker/dockerfile:1.7

FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        build-essential \
        ca-certificates \
        cmake \
        coreutils \
        curl \
        diffutils \
        dnsutils \
        file \
        findutils \
        gawk \
        git \
        grep \
        gzip \
        iproute2 \
        less \
        libbz2-dev \
        libffi-dev \
        liblzma-dev \
        libreadline-dev \
        libsqlite3-dev \
        libssl-dev \
        lsof \
        make \
        netcat-openbsd \
        ninja-build \
        openssh-client \
        openssl \
        patch \
        pkg-config \
        procps \
        psmisc \
        rsync \
        sed \
        sqlite3 \
        tar \
        tini \
        tree \
        unzip \
        wget \
        xz-utils \
        zip \
        zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 kagen \
    && mkdir -p /projects /opt/kagen \
    && chown -R kagen:kagen /projects /opt/kagen

ENV HOME=/home/kagen

WORKDIR /projects/workspace

USER kagen

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["sleep", "infinity"]
