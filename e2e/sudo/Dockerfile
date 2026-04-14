FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends sudo && rm -rf /var/lib/apt/lists/*

# Create a non-root user with NOPASSWD sudo access.
RUN useradd -m -s /bin/bash testuser \
    && echo "testuser ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/testuser \
    && chmod 0440 /etc/sudoers.d/testuser

WORKDIR /src

# Cache Go module downloads; source code is bind-mounted at runtime via -v.
COPY go.mod go.sum ./
RUN go mod download
