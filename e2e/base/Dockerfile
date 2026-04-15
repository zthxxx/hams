# hams per-provider integration-test base image.
#
# Contract: debian-slim + the minimum tools every provider's integration.sh
# needs to run. Deliberately excludes language toolchains (python, node,
# go, rust) and package managers beyond apt-get — each provider's
# integration.sh is responsible for installing its own runtime, because
# that is what hams must do for real users.
#
# Rebuild only when THIS file changes. The ci:itest:run task keys the
# image tag on the sha256 of this file; `docker image inspect` gates
# the rebuild so subsequent runs reuse the cached image.

FROM debian:bookworm-slim

# yq (Mike Farah's Go implementation) — YAML assertions in
# e2e/base/lib/yaml_assert.sh. Pinned from the official GitHub release
# to avoid the Debian repo's older, differently-behaving variant.
ARG YQ_VERSION=v4.44.3

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl bash git sudo \
 && ARCH="$(dpkg --print-architecture)" \
 && curl -fsSL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH}" \
      -o /usr/local/bin/yq \
 && chmod +x /usr/local/bin/yq \
 && rm -rf /var/lib/apt/lists/*

# Runtime contract (bind-mounted by the test harness):
#   /usr/local/bin/hams           -> the real hams binary, read-only
#   /e2e/base/lib/                -> assertions.sh, yaml_assert.sh, provider_flow.sh
#   /integration/                 -> the provider's own integration/ directory
#                                   (integration.sh + per-provider Dockerfile)
WORKDIR /workspace
