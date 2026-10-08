# Optional Claude development runtime

This is a thin adaptation of Microsoft's maintained Ubuntu 26.04 devcontainer base. The operator accepts
administrator-supplied digest-pinned runtime images; this candidate is separate from manager/hook/chart
releases. Runtime CI builds privately, runs functional and security checks, retains SBOM/scan metadata and
publishes no vendor image. This candidate is not approved for deployment.

## Contents and requirements

| Component | Source and purpose |
| --- | --- |
| Ubuntu 26.04 devcontainer | Digest-pinned Microsoft base; includes Bash, Git and development utilities. |
| Claude 2.1.285 | Native binary with signing-key fingerprint, manifest signature and architecture checksum verification. Required by both orchestrator and session runner. |
| Node 24 / npm 11.21 / Yarn | Digest-pinned official Node SDK plus a SHA-512-pinned npm release and two temporary upstream dependency overrides. Project tooling, not native Claude dependencies. |
| Python 3.14 / pip / venv | Ubuntu packages for Python projects; uses Ubuntu's maintained interpreter and dependency packages. |
| curl / libcurl / OpenSSH client | Ubuntu packages and their dependency graph. No source builds, private `.deb` packages or hand-maintained ABI variants. |
| ripgrep | Ubuntu package for shell/search workflows. |

The base and Node SDK are pinned by OCI index digest for AMD64/ARM64. Apt uses Ubuntu's signed current
update/security repositories during rebuilds, rather than a frozen Debian snapshot. Installed package
versions can advance between builds; scan and promote the resulting image by digest. Updating base/SDK
pins or accepting a rebuilt image requires review and testing. Original base, SDK and package notices
remain in the image. The unused inherited Pebble supervisor is removed.

npm 11.21.0 currently bundles vulnerable brace-expansion/undici releases. The build temporarily replaces
only those two dependencies with checksum-pinned upstream 5.0.11/6.28.1 packages, checks actual parent
dependency ranges and prerequisites, rejects unsafe archives, and retains original package licenses.
The applied versions/checksums/advisories are recorded in `/usr/local/share/claude-runtime/npm-security-overrides.json`.
Remove these overrides when a verified npm release includes the fixes; curl/OpenSSH remain distro packages.

The SDK profile retains Node/Python for compatibility with general development tasks. Python uses
Ubuntu's maintained 3.14 series; the previous Ubuntu 24.04 candidate used 3.12. Test repository-specific
dependencies before choosing this image. Add other compilers/SDKs only when a repository requires them,
preferably in a downstream image.
[Devcontainer base](https://github.com/devcontainers/images/tree/main/src/base-ubuntu),
[Claude runner image requirements](https://code.claude.com/docs/en/self-hosted-environments-deploy#build-the-runner-image).

The base's UID/GID 1000 account is renamed to `runner`, moved to `/home/runner`, removed from supplementary
groups and stripped of its passwordless sudo grant. Claude is installed globally at `/usr/local/bin/claude`.
Root remains read-only at execution; `/workspace`, `/home/runner` and `/tmp` are the writable volumes.
Automatic and manual Claude updates are disabled. The operator runs Claude directly and does not process
`devcontainer.json`, Features, Coder agents or `postCreateCommand` at session startup.

## Build and verify

Run from the repository worktree on matching native hardware:

```sh
docker build --platform linux/arm64 -f images/runtime/Dockerfile -t claude-cloud-runtime:dev .
sh hack/runtime-smoke.sh claude-cloud-runtime:dev linux/arm64
sh hack/runtime-smoke.sh claude-cloud-runtime:dev linux/arm64 security
```

Use `linux/amd64` on AMD64 hardware. Emulation can build images but does not certify Node/Bun execution.
Functional smoke checks the restricted user/filesystem, verified Claude version and hook flags, Python
SSL/compression/SQLite/venv/pip, offline npm package installation, SSH signing/tamper rejection/agent cleanup
and Git HTTPS clone/commit/push with certificate validation. Separate security probes retain libcurl
credential/Digest/cookie controls and npm dependency regressions. They are acceptance gates, not optional
waivers. CI still collects the inventory and enforces the Critical threshold when a smoke/probe fails.

The Ubuntu 26.04 native ARM64 build passes functional and security smoke, including libcurl credential,
Digest and cookie isolation plus npm brace/WebSocket regressions. Each npm probe runs in a separate
process; curl probe groups and the npm cache review still run after another check fails. The retired
Ubuntu 24.04 image reproduces failures under the same expanded suite. Native AMD64 verification and
new inventory results are checked in CI before claiming that architecture or scan clearance.

The unpatched `http-cache-semantics` package advisory remains visible. The installed npm cache caller
uses `shared: false`; offline integration verifies no-store behavior, omission of Set-Cookie from cached
responses and independent cache paths. The operator gives every runner its own ephemeral home/cache.
This supports a limited assessment of npm's default use, not a library fix or a scanner suppression.
Do not share caches across sessions/identities or use the bundled library as a shared HTTP proxy.
See the [current review and historical evidence](../../wiki/testing/runtime-vulnerability-review.md).

Real Claude registration/session/OAuth/recovery and enforced-network acceptance remain separate gates.
Claude credentials are mounted at runtime; never bake keys, tokens, OAuth login or cloud identity into
this image. The operator's Apache-2.0 license does not license the vendor binary; intended redistribution
needs its own review. The former custom curl/OpenSSH/npm work is historical evidence, recoverable from Git.
