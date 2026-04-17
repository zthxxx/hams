FROM golang:1.26-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Cache Go module downloads; source code is bind-mounted at runtime via -v.
COPY go.mod go.sum ./
RUN go mod download
