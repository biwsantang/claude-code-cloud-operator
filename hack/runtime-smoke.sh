#!/bin/sh
# Local private-image check; never registers a runner or publishes vendor software.
set -eu
image=${1:?provide the privately built runtime image}
platform=${2:?provide linux/amd64 or linux/arm64}
test "$(docker image inspect --format '{{.Config.User}}' "$image")" = '1000:1000'
docker run --rm --platform "$platform" --read-only --cap-drop ALL \
  --security-opt no-new-privileges --tmpfs /tmp:rw,nosuid,nodev,uid=1000,gid=1000,size=64m \
  --tmpfs /home/runner:rw,nosuid,nodev,uid=1000,gid=1000,size=64m \
  --entrypoint /bin/sh "$image" -eu -c '
    test "$(id -u)" = 1000
    test "$DISABLE_AUTOUPDATER" = 1
    test "$SELF_HOSTED_RUNNER_HOST_CONFIG_DIR" = /etc/claude
    test "$(claude --version)" = "2.1.285 (Claude Code)"
    node --version
    python3 --version
    git --version
    claude self-hosted-runner --help > /tmp/runner-help
    claude self-hosted-runner orchestrator --help > /tmp/orchestrator-help
    for flag in --capacity --confine-repo-settings --use-anthropic-git-proxy --configure-git; do
      rg -q -- "$flag" /tmp/runner-help
    done
    for flag in --hooks-dir --hook-timeout --expected-spawn-seconds --min-idle; do
      rg -q -- "$flag" /tmp/orchestrator-help
    done
    printf "%s\n" "PASS: non-root read-only runtime, pinned CLI, tools, hook flags and updater disabled"
  '
