# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Private candidate: vendor redistribution and real session acceptance remain separate gates.
FROM node:24-bookworm-slim@sha256:d6aa754f16b3197301076f047b5def2f02ea1dbbc2ca920407d46d7ec7f87b20 AS node
FROM mcr.microsoft.com/devcontainers/base:ubuntu26.04@sha256:edfb983aab9c579a385dc23c57d7d3703f5ec920124d99c16204a2cac465aab4 AS tools
USER root
# Keep the official Node SDK and its original notices; Python and OS tools are Ubuntu packages.
COPY --from=node /usr/local/ /usr/local/
COPY --from=node /opt/ /opt/
# Claude runs directly; remove the unused inherited Pebble supervisor.
RUN apt-get -o APT::Update::Error-Mode=any update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends ca-certificates curl openssh-client ripgrep \
      python3 python3-venv python3-pip libatomic1 libstdc++6 && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    rm -f /usr/bin/pebble && \
    usermod --login runner --home /home/runner --move-home vscode && groupmod --new-name runner vscode && \
    usermod -G '' runner && rm -f /etc/sudoers.d/vscode && \
    mkdir -p /workspace /etc/claude && chown 1000:1000 /workspace /home/runner
# Keep npm pinned; remove the two verified upstream overrides when its bundle includes the fixes.
COPY images/runtime/install-npm.sh /tmp/install-npm.sh
COPY images/runtime/install-npm-overrides.py /tmp/install-npm-overrides.py
RUN sh /tmp/install-npm.sh && python3 /tmp/install-npm-overrides.py && \
    rm /tmp/install-npm.sh /tmp/install-npm-overrides.py
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
