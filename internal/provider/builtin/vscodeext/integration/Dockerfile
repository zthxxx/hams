# vscodeext provider integration image. Installs the VS Code CLI (the
# standalone `code` binary, not the full IDE) from Microsoft's official
# download.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

RUN ARCH="$(dpkg --print-architecture)" \
 && case "$ARCH" in \
      amd64) CLI_ARCH="cli-alpine-x64" ;; \
      arm64) CLI_ARCH="cli-alpine-arm64" ;; \
      *) echo "unsupported arch: $ARCH" && exit 1 ;; \
    esac \
 && curl -fsSL "https://update.code.visualstudio.com/latest/${CLI_ARCH}/stable" \
      -o /tmp/code-cli.tar.gz \
 && tar -xzf /tmp/code-cli.tar.gz -C /usr/local/bin/ \
 && rm /tmp/code-cli.tar.gz \
 && code --version
