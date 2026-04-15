# homebrew provider integration image (Linuxbrew).
#
# Linuxbrew refuses to run as root. We create a non-root `brew` user with
# passwordless sudo; integration.sh runs the actual test flow as that user
# via `sudo -u brew -i`. The tests keep working under user-said-"root for
# simplicity" because the OUTER process is root inside the container —
# the brew user is just a workaround for linuxbrew's own restriction.

ARG BASE=hams-itest-base:latest
FROM ${BASE}

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential procps file locales \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -m -s /bin/bash brew \
 && echo "brew ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/brew \
 && chmod 440 /etc/sudoers.d/brew

USER brew
WORKDIR /home/brew

RUN NONINTERACTIVE=1 /bin/bash -c \
    "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" \
 && echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> /home/brew/.bashrc \
 && /home/linuxbrew/.linuxbrew/bin/brew --version

USER root
ENV PATH="/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:${PATH}"
