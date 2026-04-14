# syntax=docker/dockerfile:1.7
#
# Default hams dev-sandbox image.
#
# Consumed by `task dev EXAMPLE=<name>`:
#   - the host builds bin/hams-linux-<arch> via the watcher and mounts
#     $(pwd)/bin into the container at /hams-bin (read-only),
#   - start-container.sh creates /usr/local/bin/hams as a symlink to
#     /hams-bin/hams-linux-<arch> after `docker run`, so this image
#     stays arch-agnostic (no uname-based wrapper inside the image).
#
# Examples may override this file by shipping their own
# examples/<name>/Dockerfile.

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

# Base toolchain for hams development scenarios. apt lists are kept primed
# with a second `apt-get update` so in-container `apt install` during a dev
# session doesn't need to refetch indexes on every invocation.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      curl \
      git \
      procps \
      sudo \
 && rm -rf /var/lib/apt/lists/*

RUN apt-get update

# Unprivileged user baked for interactive shells. `docker run --user <uid>`
# overrides this with the host uid/gid so bind-mounted files land with
# correct ownership on the host. HOME=/home/dev is forced via -e so hams
# config paths resolve regardless of whether the host uid has a matching
# /etc/passwd entry inside the container.
RUN groupadd --system --gid 1000 dev \
 && useradd --system --uid 1000 --gid 1000 --shell /bin/bash --create-home dev \
 && chmod 0777 /home/dev

# Dev-sandbox sudoers: any uid (including host uids without /etc/passwd
# entries) gets passwordless sudo. Acceptable because the container is
# ephemeral (--rm), has no network services exposed, and exists only to
# run hams scenarios.
RUN printf 'ALL ALL=(ALL) NOPASSWD: ALL\n' > /etc/sudoers.d/dev-sandbox \
 && chmod 0440 /etc/sudoers.d/dev-sandbox

WORKDIR /workspace
