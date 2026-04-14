FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl bash git

COPY bin/hams-linux-amd64 /usr/local/bin/hams
COPY e2e/lib/ /e2e/lib/
COPY e2e/alpine/run-tests.sh /run-tests.sh
RUN chmod +x /usr/local/bin/hams /run-tests.sh

WORKDIR /workspace

CMD ["/run-tests.sh"]
