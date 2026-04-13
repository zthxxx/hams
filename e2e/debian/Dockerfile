FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash git \
    && rm -rf /var/lib/apt/lists/*

COPY bin/hams-linux-amd64 /usr/local/bin/hams
RUN chmod +x /usr/local/bin/hams

COPY e2e/run-tests.sh /run-tests.sh
RUN chmod +x /run-tests.sh

WORKDIR /workspace

CMD ["/run-tests.sh"]
