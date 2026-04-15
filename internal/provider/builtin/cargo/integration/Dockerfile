# cargo provider integration image. Installs rustup + cargo non-interactively.
# Runtime install lives here (cached by content hash) so test runs don't pay
# the ~1min rustup cost every invocation.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

# Minimal build toolchain (gcc etc.) — cargo compiles native code.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential pkg-config libssl-dev \
 && rm -rf /var/lib/apt/lists/*

# rustup installs cargo + stable toolchain into /root/.cargo.
ENV CARGO_HOME=/root/.cargo
ENV RUSTUP_HOME=/root/.rustup
ENV PATH="/root/.cargo/bin:${PATH}"
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
     | sh -s -- -y --default-toolchain stable --profile minimal \
 && cargo --version
