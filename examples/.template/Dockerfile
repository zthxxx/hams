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

# Unprivileged user that matches the host uid/gid at runtime via
# `docker run --user`. The baked 1000:1000 is a convenience for shells
# started without --user.
RUN groupadd --system --gid 1000 dev \
 && useradd --system --uid 1000 --gid 1000 --shell /bin/bash --create-home dev \
 && printf 'dev ALL=(ALL) NOPASSWD: ALL\n' > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

WORKDIR /workspace
