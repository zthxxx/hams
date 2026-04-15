FROM debian:bookworm-slim

# yq (Mike Farah's Go implementation) powers the YAML assertions in
# e2e/base/lib/yaml_assert.sh. Installed from the official GitHub release to avoid
# the Debian repo's older packaging of a different tool.
ARG YQ_VERSION=v4.44.3
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash git sudo \
 && ARCH="$(dpkg --print-architecture)" \
 && curl -fsSL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH}" \
      -o /usr/local/bin/yq \
 && chmod +x /usr/local/bin/yq \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Binary, test scripts, fixtures, and configs are bind-mounted at runtime.
# See .github/workflows/ci.yml for the docker run -v flags.
