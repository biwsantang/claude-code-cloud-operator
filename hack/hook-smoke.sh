#!/bin/sh
# Match the orchestrator init-container identity, posture and hook volume budget.
set -eu
image=${1:?provide the privately built hook image}
platform=${2:?provide linux/amd64 or linux/arm64}
test "$(docker image inspect --format '{{.Config.User}}' "$image")" = '1000:1000'
docker run --rm --platform "$platform" --read-only --cap-drop ALL \
  --security-opt no-new-privileges \
  --tmpfs /hooks:rw,nosuid,nodev,uid=1000,gid=1000,size=32m \
  "$image" install /hooks
printf '%s\n' "PASS: non-root read-only hook installs within the 32Mi hook budget"
