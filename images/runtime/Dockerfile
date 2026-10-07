# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Private candidate: vendor redistribution and real session acceptance remain separate gates.
FROM node:24-bookworm-slim@sha256:d6aa754f16b3197301076f047b5def2f02ea1dbbc2ca920407d46d7ec7f87b20 AS node
FROM mcr.microsoft.com/devcontainers/base:ubuntu24.04@sha256:d7c468679f45a52ad3673d06656b5bf16e488990b17216a1fa295d9e1f89d724 AS tools
USER root
# Keep the official Node SDK and its original notices; Python and OS tools are Ubuntu packages.
COPY --from=node /usr/local/ /usr/local/
COPY --from=node /opt/ /opt/
RUN apt-get -o APT::Update::Error-Mode=any update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends ca-certificates curl openssh-client ripgrep \
      python3 python3-venv python3-pip libatomic1 libstdc++6 && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    usermod --login runner --home /home/runner --move-home vscode && groupmod --new-name runner vscode && \
    usermod -G '' runner && rm -f /etc/sudoers.d/vscode && \
    mkdir -p /workspace /etc/claude && chown 1000:1000 /workspace /home/runner
# A standard, checksum-verified npm release; no edits to its bundled dependencies.
COPY images/runtime/install-npm.sh /tmp/install-npm.sh
RUN sh /tmp/install-npm.sh && rm /tmp/install-npm.sh
FROM tools AS native
ARG TARGETARCH
RUN apt-get -o APT::Update::Error-Mode=any update && \
    apt-get install -y --no-install-recommends gnupg jq
COPY images/runtime/install-native.sh /tmp/install-native.sh
RUN sh /tmp/install-native.sh "$TARGETARCH"
FROM tools AS runtime
COPY --from=native /usr/local/bin/claude /usr/local/bin/claude
ENV HOME=/home/runner DISABLE_AUTOUPDATER=1 DISABLE_UPDATES=1 SELF_HOSTED_RUNNER_HOST_CONFIG_DIR=/etc/claude
USER 1000:1000
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/claude"]
