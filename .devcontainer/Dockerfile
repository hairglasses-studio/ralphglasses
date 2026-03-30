FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

ARG GO_VERSION=1.26.1

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash \
      bats \
      build-essential \
      ca-certificates \
      curl \
      git \
      make \
      python3 \
      shellcheck \
      tar && \
    rm -rf /var/lib/apt/lists/*

# Install Go
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/home/vscode/go/bin:${PATH}"

# Install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin
