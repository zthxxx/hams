FROM alpine:3.20

# Simulate OpenWrt-like minimal environment (Alpine-based, musl libc).
# Real OpenWrt uses opkg, but for testing hams on musl/ARM64 this is sufficient.
RUN apk add --no-cache ca-certificates bash git

WORKDIR /workspace

# Binary, test scripts, fixtures, and configs are bind-mounted at runtime.
# See .github/workflows/ci.yml for the docker run -v flags.
