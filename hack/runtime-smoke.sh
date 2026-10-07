#!/bin/sh
set -eu
image=${1:?provide the private runtime image}
platform=${2:?provide linux/amd64 or linux/arm64}
mode=${3:-functional}
case "$platform" in linux/amd64|linux/arm64) ;; *) exit 2 ;; esac
case "$mode" in functional|security) ;; *) exit 2 ;; esac
test "$(docker image inspect --format '{{.Config.User}}' "$image")" = '1000:1000'
checks_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

# Run on matching physical architecture. QEMU does not certify Node/Bun CPU compatibility.
docker run --rm --platform "$platform" --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --entrypoint /bin/sh \
  --mount "type=bind,src=$checks_dir/npm-security-smoke.cjs,dst=/checks/npm-security-smoke.cjs,readonly" \
  --mount "type=bind,src=$checks_dir/runtime-python-smoke.py,dst=/checks/runtime-python-smoke.py,readonly" \
  --mount "type=bind,src=$checks_dir/runtime-ssh-smoke.py,dst=/checks/runtime-ssh-smoke.py,readonly" \
  --mount "type=bind,src=$checks_dir/runtime-curl-smoke.py,dst=/checks/runtime-curl-smoke.py,readonly" \
  --tmpfs /tmp:rw,nosuid,nodev,size=64m \
  --tmpfs /home/runner:rw,nosuid,nodev,size=64m,uid=1000,gid=1000 \
  "$image" -c '
    set -eu
    test "$(id -u)" = 1000
    test "$DISABLE_AUTOUPDATER" = 1
    test "$DISABLE_UPDATES" = 1
    test "$SELF_HOSTED_RUNNER_HOST_CONFIG_DIR" = /etc/claude
    test ! -e /var/run/secrets/kubernetes.io/serviceaccount/token
    test "$(node --version)" = v24.21.0
    test "$(npm --version)" = 11.21.0
    test "$(npx --version)" = 11.21.0
    test "$(yarn --version)" = 1.22.22
    test "$(getent passwd 1000 | cut -d: -f1)" = runner
    test "$(getent passwd 1000 | cut -d: -f6)" = /home/runner
    if sudo -n true 2>/dev/null; then exit 1; fi
    if [ "$1" = security ]; then
      result=0
      python3 /checks/runtime-curl-smoke.py --security-only || result=1
      node /checks/npm-security-smoke.cjs || result=1
      exit "$result"
    fi
    test "$(claude --version)" = "2.1.285 (Claude Code)"
    git --version
    python3 --version
    rg --version
    curl --version
    ssh -V
    python3 /checks/runtime-python-smoke.py
    python3 /checks/runtime-ssh-smoke.py
    python3 /checks/runtime-curl-smoke.py --sdk-only
    claude self-hosted-runner --help > /tmp/runner-help
    claude self-hosted-runner orchestrator --help > /tmp/orchestrator-help
    for flag in --capacity --confine-repo-settings --use-anthropic-git-proxy --configure-git; do
      rg -q -- "$flag" /tmp/runner-help
    done
    for flag in --hooks-dir --hook-timeout --expected-spawn-seconds --min-idle; do
      rg -q -- "$flag" /tmp/orchestrator-help
    done
    mkdir -p /tmp/npm-smoke/dependency /tmp/npm-smoke/project
    cd /tmp/npm-smoke/dependency
    printf "%s\n" "{\"name\":\"synthetic-dependency\",\"version\":\"1.0.0\"}" > package.json
    printf "%s\n" "module.exports = 17" > index.js
    npm pack --ignore-scripts --pack-destination /tmp/npm-smoke
    cd /tmp/npm-smoke/project
    printf "%s\n" "{\"name\":\"synthetic-offline\",\"version\":\"1.0.0\",\"private\":true,\"dependencies\":{\"synthetic-dependency\":\"file:../synthetic-dependency-1.0.0.tgz\"}}" > package.json
    npm install --offline --ignore-scripts --no-audit --no-fund
    node -e "require(\"assert\").strictEqual(require(\"synthetic-dependency\"), 17)"
    test -f package-lock.json
    printf "%s\n" "PASS: non-root read-only runtime, pinned CLI/tools, hook flags, offline npm and Python venv/pip"
  ' runtime-smoke "$mode"
