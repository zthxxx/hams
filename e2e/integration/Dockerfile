FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Run only integration-tagged tests
CMD ["go", "test", "-v", "-tags=integration", "-race", "-count=1", "./..."]
