# uv provider integration image. Installs astral-sh/uv via the official
# installer script; uv manages its own Python runtimes.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

RUN curl -LsSf --retry 5 --retry-delay 2 --retry-all-errors \
      https://astral.sh/uv/install.sh | sh \
 && /root/.local/bin/uv --version

ENV PATH="/root/.local/bin:${PATH}"
