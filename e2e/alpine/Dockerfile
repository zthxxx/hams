FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl bash git

WORKDIR /workspace

# Binary, test scripts, fixtures, and configs are bind-mounted at runtime.
# See .github/workflows/ci.yml for the docker run -v flags.
