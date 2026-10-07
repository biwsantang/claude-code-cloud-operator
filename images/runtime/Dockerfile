# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Build privately for authorized use. Vendor redistribution is a separate release gate.
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS certificates
FROM node:24-bookworm-slim@sha256:d6aa754f16b3197301076f047b5def2f02ea1dbbc2ca920407d46d7ec7f87b20 AS tools
# The slim base has no CA bundle. Bootstrap TLS from another digest-pinned image before apt.
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN printf '%s\n' 'deb [check-valid-until=no] https://snapshot.debian.org/archive/debian/20261007T000000Z bookworm main' \
    'deb [check-valid-until=no] https://snapshot.debian.org/archive/debian-security/20261007T000000Z bookworm-security main' > /etc/apt/sources.list && \
    rm /etc/apt/sources.list.d/debian.sources && apt-get -o APT::Update::Error-Mode=any update && \
    apt-get install -y --no-install-recommends ca-certificates curl git python3 python3-venv ripgrep bash openssh-client && \
    apt-get clean && mkdir -p /workspace /home/runner /etc/claude && chown -R 1000:1000 /workspace /home/runner
FROM tools AS native
ARG TARGETARCH
RUN apt-get install -y --no-install-recommends gnupg jq
COPY images/runtime/install-native.sh /tmp/install-native.sh
RUN sh /tmp/install-native.sh "$TARGETARCH"
FROM tools AS runtime
COPY --from=native /usr/local/bin/claude /usr/local/bin/claude
ENV HOME=/home/runner DISABLE_AUTOUPDATER=1 SELF_HOSTED_RUNNER_HOST_CONFIG_DIR=/etc/claude
USER 1000:1000
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/claude"]
