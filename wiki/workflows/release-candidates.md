---
type: Repository Documentation
title: "Private release candidate verification"
description: "Build, catalogue and verify review artifacts before selecting a publishing identity."
tags: [claude-code, release, supply-chain]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T15:52:00+07:00"
sources:
  - resource: repo://hack/build-candidate.py
  - resource: repo://hack/verify-candidate.py
  - resource: repo://hack/candidate-smoke.py
  - resource: repo://hack/collect-notices.py
  - resource: repo://.github/workflows/test.yml
  - resource: https://github.com/golang/vuln/tree/v1.8.0
  - resource: https://github.com/anchore/syft/releases/tag/v1.54.1
  - resource: https://github.com/sigstore/cosign/releases/tag/v3.1.3
  - resource: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations
  - resource: https://go.dev/ref/mod#go-mod-verify
  - resource: https://www.apache.org/licenses/LICENSE-2.0
---

# Build a private review bundle

This advances release preparation in [task 7.3](../changes/build-kubernetes-operator/tasks.md).
It does not complete release acceptance, publish images or include the vendor runtime. Keep the full
native image, licensing, support, signing-identity and integration gates in the plan.

The producer builds manager and hook binaries for Linux AMD64/ARM64 with the exact Go version in
`go.mod`, CGO disabled, trimmed paths and embedded VCS metadata. It runs the source vulnerability scan
for each target architecture, catalogues each compiled binary into a CycloneDX SBOM, and checks that every
compiled dependency in Go build information appears in the catalogue. The bundle includes build information,
scan output, raw installation resources, a chart archive and the source license. Chart installation still
requires a separately verified manager image digest; its empty default is intentional.

The producer also includes `third-party-notices.tar.gz` and `third-party-notices.json` in the signed inventory.
The [notice collector](../../hack/collect-notices.py) derives the module/version/checksum union from actual
compiled build information, verifies each download matches that identity, and runs
[`go mod verify`](https://go.dev/ref/mod#go-mod-verify) before and after collection to detect modified cached
module source. It preserves original license, notice, patent, copyright and author files recursively, including
variant names such as `LICENSE-MIT`, plus the selected compiler's notice files. The archive has normalized
metadata; the index binds original source paths, bytes/hashes and the binaries using each module without
embedding local cache paths. Missing license files, replacement-module provenance, inconsistent compiler or
checksum identities, modules outside the current unreplaced build list, symlinks and oversized inputs fail collection.

This deliberately includes whole module/toolchain trees, so it can include notices for uncompiled code.
Filename discovery is not semantic license analysis: reviewers must check relevant source headers, additional
terms and distribution obligations. The index always records `licenseReviewApproved: false` and excludes the
vendor runtime. The [Apache license's redistribution conditions](https://www.apache.org/licenses/LICENSE-2.0)
include retaining applicable notices; the repository's own license cannot substitute for dependency terms.
Review container OS licenses and [vendor distribution/public naming](../integrations/anthropic-contract.md)
separately before publishing. Neither an SBOM nor a collected notice archive proves licensing approval.

`candidate.json` binds every file's SHA-256 and size, the source commit/tree digest, tool versions and dirty
source flag. It always records `releaseApproved: false` and `vendorRuntimeIncluded: false`. The source
snapshot must remain unchanged during the build. Output must be outside the worktree and a new directory.
Dirty source is rejected unless `--allow-dirty` explicitly requests an uncommitted review candidate.

Use verified Syft 1.54.1 and Cosign 3.1.3 executables. Their upstream asset pins are:

| Tool asset | SHA-256 |
| --- | --- |
| Syft 1.54.1 darwin_arm64 tar.gz | `b4319c3abaa87a0170ab76ee83ea2260ca34b53aecfa3ab0dd5428d2319d744f` |
| Syft 1.54.1 linux_amd64 tar.gz | `c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47` |
| Cosign 3.1.3 darwin-arm64 binary | `5cf948c2f4dfe59687bdd0b8523709067383e03982cc543475c8a7dc70e92a76` |
| Cosign 3.1.3 linux-amd64 binary | `4629c757b7618056f8ddd7e2625ae9fdd94c0372a65049520bc7d9df9efc7f71` |

Govulncheck module tag `v1.8.0` resolves to upstream commit
`709015412431dd2b5b28a53c06c70bc02d49074c`. Build it with the pinned project toolchain:

```sh
GOTOOLCHAIN=go1.27.1 GOBIN=PATH_TO_TOOLS go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
python3 hack/build-candidate.py --output /tmp/claude-review-UNIQUE \
  --syft PATH_TO_VERIFIED_SYFT --govulncheck PATH_TO_GOVULNCHECK
```

The CLI selects that project compiler for both Go builds and the scanner's subprocesses. A PATH launcher
with an older global Go version must not silently select the scanner's analysis version. The older GitHub
release page identified v1.1.4; it failed to analyze Go 1.27 syntax even after rebuilding its executable.
The current Go module tag and analysis libraries were verified independently, then full source scans passed.

The [scanner documentation](https://github.com/golang/vuln/blob/v1.8.0/cmd/govulncheck/doc.go) explains
that JSON/SARIF output exits zero even with findings. CI and the producer use text mode so findings and
scan errors fail the gate. These checks cover known reachable Go vulnerabilities for the selected build;
reflection/unsafe limitations, container OS packages, the vendor binary and future advisories need separate
review. An SBOM is inventory evidence, not a license or vulnerability clearance.

## Private signing policy

[GitHub's artifact attestation feature](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)
requires Enterprise Cloud for private/internal repositories. Do not change repository visibility or assume
that feature is available. This candidate path supports self-managed/KMS signing keys with an independently
distributed public key; the release identity and key management remain an acceptance decision.
Native environment keys, session/OAuth credentials and Anthropic's JWT verification keys serve separate
authentication purposes and cannot substitute for this release signing identity.

After that identity is selected, sign the manifest, which binds the complete inventory:

```sh
PATH_TO_COSIGN sign-blob --key PATH_OR_KMS_REFERENCE \
  --use-signing-config=false --tlog-upload=false --yes \
  --bundle /tmp/claude-review-UNIQUE/candidate.sigstore.json \
  /tmp/claude-review-UNIQUE/candidate.json
python3 hack/verify-candidate.py --directory /tmp/claude-review-UNIQUE \
  --cosign PATH_TO_VERIFIED_COSIGN --public-key INDEPENDENTLY_TRUSTED_PUBLIC_KEY
```

This offline key policy has no public transparency-log inclusion, certificate identity or timestamp evidence.
The verifier explicitly verifies with the supplied trusted key and disables log verification for this policy;
it has no keyless fallback. A key inside the candidate directory is rejected. Key distribution, rotation,
revocation and publication approval must be resolved before a production release.

The verifier checks the signature before parsing the manifest, then checks every file's size/hash and rejects
missing/unlisted files, symlinks, unsafe names and unknown schemas. Verification proves integrity relative to
the chosen trusted key; it does not prove source claims independently, licensing, safety or deployment approval.

## Reproduce the signing exercise

```sh
python3 hack/candidate-smoke.py --directory /tmp/claude-review-UNIQUE \
  --cosign PATH_TO_VERIFIED_COSIGN --evidence /tmp/claude-candidate-smoke.json
```

The fixture copies the candidate, generates ephemeral test-only keys in a private temporary directory and
removes them on completion. It verifies the real Cosign signing path, then rejects a wrong trust key,
modified manifest, same-size binary change, missing/extra files, symlinks and a candidate-provided key.
It never signs the original candidate or creates a production key. [Implementation evidence](../testing/implementation-evidence.md)
records observed results and their source state. `make test-candidate` runs independent inventory-boundary
unit tests and notice/provenance boundaries; `make verify` includes them. Publication and real vendor acceptance remain pending.
