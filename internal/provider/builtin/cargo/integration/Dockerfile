# cargo provider integration image. Uses the official `rust` image so
# cargo is pre-installed (modern version with sparse registry support).
# Re-bases on rust:1-slim-bookworm (debian bookworm, matches our other
# provider images) then re-applies the shared base contract (yq + sudo).
# Departure from the shared-base pattern is accepted here because:
#   (a) apt's cargo/rustc is stuck at 1.63 and can't clone crates.io-index
#       efficiently (10+ min hangs on "Updating crates.io index"), and
#   (b) rustup's self-download stalls on flaky networks with no retry.
# The official rust image is maintained, hashed, and ready to use.

FROM rust:1-slim-bookworm

ARG YQ_VERSION=v4.44.3

# Re-install the base-image contract (yq, sudo, curl, ca-certs, git) so
# /e2e/base/lib/* still works and integration.sh can assume them.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl bash git sudo pkg-config libssl-dev \
 && ARCH="$(dpkg --print-architecture)" \
 && curl -fsSL --retry 5 --retry-delay 2 --retry-all-errors \
      "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH}" \
      -o /usr/local/bin/yq \
 && chmod +x /usr/local/bin/yq \
 && rm -rf /var/lib/apt/lists/* \
 && cargo --version

# Use the sparse crates.io registry — dramatically faster first-install
# than the legacy git-based index.
ENV CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse

WORKDIR /workspace
