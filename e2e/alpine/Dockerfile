FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl bash

COPY bin/hams-linux-amd64 /usr/local/bin/hams
RUN chmod +x /usr/local/bin/hams

WORKDIR /workspace

CMD ["hams", "--version"]
