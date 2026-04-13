FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash \
    && rm -rf /var/lib/apt/lists/*

COPY bin/hams-linux-amd64 /usr/local/bin/hams
RUN chmod +x /usr/local/bin/hams

WORKDIR /workspace

CMD ["hams", "--version"]
