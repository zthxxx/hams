# vscodeext provider integration image. Installs the full VS Code IDE
# from Microsoft's official apt repo. The standalone `code` tunnel CLI
# (cli-alpine-<arch>) only supports `code tunnel` / `code serve-web`
# and can't drive `--install-extension` — extension management requires
# a real IDE installation that owns ~/.vscode/extensions.

ARG BASE=hams-itest-base:latest
FROM ${BASE}
RUN apt-get update && apt-get install -y --no-install-recommends \
      gnupg wget \
 && wget -qO- https://packages.microsoft.com/keys/microsoft.asc \
      | gpg --dearmor -o /etc/apt/keyrings/microsoft.gpg \
 && echo "deb [arch=amd64,arm64,armhf signed-by=/etc/apt/keyrings/microsoft.gpg] https://packages.microsoft.com/repos/code stable main" \
      > /etc/apt/sources.list.d/vscode.list \
 && apt-get update && apt-get install -y --no-install-recommends code \
 && rm -rf /var/lib/apt/lists/* \
 && code --no-sandbox --user-data-dir=/tmp/vscode-init --version | head -1

# VS Code refuses to run as root without explicit flags. The hams
# provider shells out to `code --install-extension <id>` directly, so
# we install a thin /usr/local/bin/code wrapper (earlier in PATH than
# /usr/bin/code) that appends --no-sandbox and --user-data-dir. The
# integration test runs as root inside the container per the design.
RUN mkdir -p /root/.vscode /root/.vscode-ext \
 && printf '#!/usr/bin/env bash\nexec /usr/bin/code --no-sandbox --user-data-dir=/root/.vscode --extensions-dir=/root/.vscode-ext "$@"\n' \
      > /usr/local/bin/code \
 && chmod +x /usr/local/bin/code

