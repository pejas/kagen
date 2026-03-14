# syntax=docker/dockerfile:1.7

ARG KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:phase1
# hadolint ignore=DL3006
FROM ${KAGEN_BASE_IMAGE}

USER root

ARG TARGETARCH
ARG MISE_VERSION=2026.3.8

ENV HOME=/home/kagen \
    MISE_DATA_DIR=/opt/mise \
    MISE_CACHE_DIR=/var/cache/mise \
    MISE_SYSTEM_CONFIG_FILE=/etc/mise/config.toml \
    PATH=/opt/mise/shims:/opt/mise/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN case "${TARGETARCH}" in \
        amd64) mise_arch="x64" ;; \
        arm64) mise_arch="arm64" ;; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && tmpdir="$(mktemp -d)" \
    && curl -fsSL "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-${mise_arch}.tar.gz" -o /tmp/mise.tar.gz \
    && tar -xzf /tmp/mise.tar.gz -C "${tmpdir}" \
    && mise_bin="$(find "${tmpdir}" -type f -name mise | head -n 1)" \
    && test -n "${mise_bin}" \
    && install -m 0755 "${mise_bin}" /usr/local/bin/mise \
    && rm -rf /tmp/mise.tar.gz "${tmpdir}"

COPY toolbox/mise.toml /etc/mise/config.toml
COPY toolbox/mise.lock /etc/mise/mise.lock

RUN mkdir -p "${MISE_DATA_DIR}" "${MISE_CACHE_DIR}" \
    && chown -R kagen:kagen /etc/mise "${MISE_DATA_DIR}" "${MISE_CACHE_DIR}"

USER kagen

# `core:java` currently does not emit lockfile URLs for linux-arm64, so
# strict `--locked` mode breaks the Debian arm64 toolbox build even though the
# version is pinned in mise.lock. We still use the checked-in lockfile to pin
# versions, but allow backend resolution where URL-complete lock data is absent.
RUN mise install --yes \
    && mise reshim

WORKDIR /projects/workspace

CMD ["tail", "-f", "/dev/null"]
