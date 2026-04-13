FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash git

COPY bin/hams-linux-amd64 /usr/local/bin/hams
COPY e2e/lib/ /e2e/lib/
COPY e2e/debian/run-tests.sh /run-tests.sh
RUN chmod +x /usr/local/bin/hams /run-tests.sh

WORKDIR /workspace

CMD ["/run-tests.sh"]
