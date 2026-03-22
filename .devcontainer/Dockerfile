FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash \
      bats \
      build-essential \
      ca-certificates \
      git \
      make \
      python3 \
      shellcheck \
      tar && \
    rm -rf /var/lib/apt/lists/*
