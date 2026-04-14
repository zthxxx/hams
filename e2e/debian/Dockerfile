FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash git \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Binary, test scripts, fixtures, and configs are bind-mounted at runtime.
# See .github/workflows/ci.yml for the docker run -v flags.
