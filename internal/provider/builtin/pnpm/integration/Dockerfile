# pnpm provider integration image. Installs Node.js from NodeSource then
# activates pnpm via corepack (the officially supported path on Node ≥16.13).

ARG BASE=hams-itest-base:latest
FROM ${BASE}

ARG NODE_MAJOR=20

RUN apt-get update && apt-get install -y --no-install-recommends \
      gnupg \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
      | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
      > /etc/apt/sources.list.d/nodesource.list \
 && apt-get update && apt-get install -y --no-install-recommends nodejs \
 && rm -rf /var/lib/apt/lists/* \
 && corepack enable \
 && corepack prepare pnpm@latest --activate \
 && pnpm --version
