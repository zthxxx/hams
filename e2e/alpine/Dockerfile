FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl bash git

COPY bin/hams-linux-amd64 /usr/local/bin/hams
RUN chmod +x /usr/local/bin/hams

COPY e2e/run-tests.sh /run-tests.sh
RUN chmod +x /run-tests.sh

WORKDIR /workspace

CMD ["/run-tests.sh"]
