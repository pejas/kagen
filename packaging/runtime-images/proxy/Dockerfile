# syntax=docker/dockerfile:1.7

ARG KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:phase1
FROM ${KAGEN_BASE_IMAGE}

USER root

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends tinyproxy \
    && rm -rf /var/lib/apt/lists/*

USER kagen

CMD ["tinyproxy", "-d", "-c", "/etc/kagen-proxy/tinyproxy.conf"]
