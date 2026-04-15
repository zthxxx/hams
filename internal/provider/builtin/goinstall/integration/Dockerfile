# goinstall provider integration image. Installs the Go toolchain into
# /usr/local/go and exposes it on PATH + GOPATH/bin.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

ARG GO_VERSION=1.24.0

RUN ARCH="$(dpkg --print-architecture)" \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" \
      | tar -C /usr/local -xz \
 && /usr/local/go/bin/go version

ENV GOPATH=/root/go
ENV PATH="/usr/local/go/bin:${GOPATH}/bin:${PATH}"
