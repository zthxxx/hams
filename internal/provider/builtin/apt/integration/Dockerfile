# Per-provider integration-test Dockerfile for the apt provider.
#
# apt-get and dpkg are part of the debian base image, so this overlay
# needs no runtime additions beyond what hams-itest-base provides. The
# `integration.sh` script runs `apt-get update` at the top to prime the
# package lists before exercising `hams apt install`.

ARG BASE=hams-itest-base:latest
FROM ${BASE}
