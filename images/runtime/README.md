# Optional Claude development runtime

This is a thin adaptation of Microsoft's maintained Ubuntu 24.04 devcontainer base. The operator accepts
administrator-supplied digest-pinned runtime images; this candidate is separate from manager/hook/chart
releases. Runtime CI builds privately, runs functional and security checks, retains SBOM/scan metadata and
publishes no vendor image. This candidate is not approved for deployment.

## Contents and requirements

| Component | Source and purpose |
| --- | --- |
| Ubuntu 24.04 devcontainer | Digest-pinned Microsoft base; includes Bash, Git and development utilities. |
| Claude 2.1.285 | Native binary with signing-key fingerprint, manifest signature and architecture checksum verification. Required by both orchestrator and session runner. |
| Node 24 / npm 11.21 / Yarn | Digest-pinned official Node SDK plus an unmodified, SHA-512-pinned npm release. Project tooling, not native Claude dependencies. |
| Python 3.12 / pip / venv | Ubuntu packages for Python projects; replaces the former custom Python 3.14 profile. |
| curl / libcurl / OpenSSH client | Ubuntu packages and their dependency graph. No source builds, private `.deb` packages or hand-maintained ABI variants. |
| ripgrep | Ubuntu package for shell/search workflows. |

The base and Node SDK are pinned by OCI index digest for AMD64/ARM64. Apt uses Ubuntu's signed current
update/security repositories during rebuilds, rather than a frozen Debian snapshot. Installed package
versions can advance between builds; scan and promote the resulting image by digest. Updating base/SDK
pins or accepting a rebuilt image requires review and testing. Original base, SDK and package notices
remain in the image. No npm bundled dependencies are replaced or patched by this repository.

The SDK profile retains Node/Python for compatibility with general development tasks. Python changes
from 3.14 to Ubuntu's maintained 3.12 series; test repository-specific dependencies before choosing this
image. Add other compilers/SDKs only when a repository requires them, preferably in a downstream image.
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

The initial native ARM64 build passes functional smoke. Security smoke fails on mismatched netrc user
credential isolation in distro libcurl and npm brace-expansion recursion. The unmodified npm release
contains brace-expansion 5.0.9 and undici 6.28.0; removal of the custom overrides does not establish their
security acceptance. See the [current review and historical evidence](../../wiki/testing/runtime-vulnerability-review.md).
Do not infer security clearance from distro package names, upstream version strings or the base's brand.

Real Claude registration/session/OAuth/recovery and enforced-network acceptance remain separate gates.
Claude credentials are mounted at runtime; never bake keys, tokens, OAuth login or cloud identity into
this image. The operator's Apache-2.0 license does not license the vendor binary; intended redistribution
needs its own review. The former custom curl/OpenSSH/npm work is historical evidence, recoverable from Git.
