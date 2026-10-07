# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Build privately for authorized use. Vendor redistribution is a separate release gate.
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS certificates
FROM node:24-trixie-slim@sha256:173f125896c3b47ddf056734c7ea789d04595a6a08769a8f78e0df642781fb66 AS node
# Keep Python's patched upstream interpreter and its complete runtime dependencies.
# Node uses the same Debian suite; retain its SDK, npm/npx, Yarn and original notices.
FROM python:3.14.8-slim-trixie@sha256:f85c5697265c178cc6887276c55fe16cf3d14ca35c3df6a5eab3b360534a55d2 AS tools
COPY --from=node /usr/local/ /usr/local/
COPY --from=node /opt/ /opt/
# Bootstrap TLS from another digest-pinned image before accessing the apt snapshot.
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN printf '%s\n' 'deb [check-valid-until=no] https://snapshot.debian.org/archive/debian/20261007T000000Z trixie main' \
    'deb [check-valid-until=no] https://snapshot.debian.org/archive/debian-security/20261007T000000Z trixie-security main' > /etc/apt/sources.list && \
    rm /etc/apt/sources.list.d/debian.sources && apt-get -o APT::Update::Error-Mode=any update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends ca-certificates curl git ripgrep bash openssh-client libatomic1 libstdc++6 && \
    apt-get clean && groupadd --gid 1000 runner && \
    useradd --uid 1000 --gid runner --shell /bin/bash --create-home runner && \
    mkdir -p /workspace /etc/claude && chown -R 1000:1000 /workspace /home/runner
COPY images/runtime/install-npm.sh /tmp/install-npm.sh
COPY images/runtime/install-npm-overrides.py /tmp/install-npm-overrides.py
RUN sh /tmp/install-npm.sh && python3 /tmp/install-npm-overrides.py && \
    rm /tmp/install-npm.sh /tmp/install-npm-overrides.py
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
