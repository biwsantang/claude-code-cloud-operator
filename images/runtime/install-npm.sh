#!/bin/sh
set -eu

# Keep npm's bundled dependencies current without changing the Node major or vendor CLI.
version=11.21.0
checksum=668bfc2a16a63677a22dd10bb63e5800bb4d9895b82f87c22d3ce37e9cd71b6c3a3121dc7f453181d94ae6e9680ae92c593fbb986514ef08bbf5c3ba46a8af3e
mkdir -p /tmp/claude-npm-update
archive=/tmp/claude-npm-update/npm.tgz
curl -fsSL --retry 3 "https://registry.npmjs.org/npm/-/npm-$version.tgz" -o "$archive"
printf '%s  %s\n' "$checksum" "$archive" | sha512sum -c -
npm install --global --prefix /usr/local --offline --ignore-scripts --no-audit --no-fund "$archive"
test "$(npm --version)" = "$version"
rm -rf /tmp/claude-npm-update /root/.npm
