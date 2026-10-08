#!/bin/sh
set -eu
# Vendor integrity procedure: https://code.claude.com/docs/en/setup#verify-the-manifest-signature
version=2.1.285
case "$1" in
  arm64) platform=linux-arm64; expected=24fac77749bed3d91365d6b6915aa4b824e14318ecb6bc17adbc192f01c9173d ;;
  amd64) platform=linux-x64; expected=33dad1ec615a2e08cc78b494f05c110e49916de2c79d78ec8799ebf46b233d29 ;;
  *) printf '%s\n' 'Unsupported architecture' >&2; exit 1 ;;
esac
base=https://downloads.claude.ai/claude-code-releases
mkdir -p /tmp/claude-native /tmp/claude-native/keyring
chmod 700 /tmp/claude-native/keyring
export GNUPGHOME=/tmp/claude-native/keyring
curl -fsSL --retry 3 https://downloads.claude.ai/keys/claude-code.asc -o /tmp/claude-native/key.asc
fingerprint=$(gpg --show-keys --with-colons /tmp/claude-native/key.asc | awk -F: '$1=="fpr" {print $10; exit}')
test "$fingerprint" = 31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE
gpg --batch --import /tmp/claude-native/key.asc
curl -fsSL --retry 3 "$base/$version/manifest.json" -o /tmp/claude-native/manifest.json
curl -fsSL --retry 3 "$base/$version/manifest.json.sig" -o /tmp/claude-native/manifest.json.sig
gpg --batch --verify /tmp/claude-native/manifest.json.sig /tmp/claude-native/manifest.json
checksum=$(jq -r --arg p "$platform" '.platforms[$p].checksum' /tmp/claude-native/manifest.json)
test "$checksum" = "$expected"
curl -fsSL --retry 3 "$base/$version/$platform/claude" -o /tmp/claude-native/claude
printf '%s  %s\n' "$expected" /tmp/claude-native/claude | sha256sum --check --strict
install -m 0555 /tmp/claude-native/claude /usr/local/bin/claude
