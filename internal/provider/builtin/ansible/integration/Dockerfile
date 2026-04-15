# ansible provider integration image. Installs ansible-playbook via apt-get.
# Runtime install lives here (cached by content hash) rather than in
# integration.sh to keep test runs fast.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

RUN apt-get update && apt-get install -y --no-install-recommends \
      ansible python3 \
 && rm -rf /var/lib/apt/lists/*
