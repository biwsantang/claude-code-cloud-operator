---
type: Repository Documentation
title: "Operator releases"
description: "Standard image and chart publishing with separate runtime acceptance."
tags: [claude-code, release, supply-chain]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T19:44:02+07:00"
sources:
  - resource: repo://.github/workflows/release.yml
  - resource: repo://.github/workflows/test.yml
  - resource: repo://.github/workflows/runtime.yml
  - resource: https://docs.docker.com/build/ci/github-actions/attestations/
  - resource: https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images
---

# Operator images and chart

The release unit is the manager image, spawn-hook image, chart and raw installation manifests. Claude's
native binary and the optional custom runtime are excluded. The former offline review-bundle producer,
custom inventory verifier and signing exercise have been removed; their evidence remains historical.
No release has been published. The new workflow must pass CI and its first tagged execution before claiming
publication evidence. Real vendor acceptance remains required before production use of a Fleet.

## Verification

Pull requests run one source job (unit/race/vet, module integrity, generated parity, notices tests,
AMD64/ARM64 cross-builds and reachable-Go vulnerability checks), one Kubernetes 1.33 API job and one
AMD64 manager/hook build/scan job. Main and tagged releases also test API versions 1.36/1.37 and ARM64
images. This is empirical compatibility coverage, not a vendor support commitment.

Image scans use the pinned Grype action/version, fail on Critical findings including unfixed findings,
and retain reports for seven days. High and lower findings remain visible for release review; do not add
blanket ignores or replace a failed scan with a success. Image SBOMs inventory dependencies; source
vulnerability checks additionally detect known reachable Go issues. Actions are pinned to commits.

The separate runtime workflow runs only on runtime-related changes or manual dispatch, retains its
existing smoke/security gates and publishes no vendor software. An operator check cannot certify the
runtime, and a runtime advisory is not automatically a vulnerability in the manager/hook.

## Publish

After the reviewed code is merged into `main`, a maintainer tags its commit `vMAJOR.MINOR.PATCH`, optionally
with a prerelease suffix such as `v0.1.0-rc.1`. The workflow rejects malformed tags and commits outside
`main` history. It repeats the full operator checks before giving the publishing jobs write permissions.

The workflow pushes `ghcr.io/OWNER/claude-code-cloud-operator-manager` and the corresponding `-hook` image
for Linux AMD64/ARM64. Version and source-commit tags identify the release; recorded OCI digests are the
installation identity. It includes standard SBOM/BuildKit provenance and original dependency/compiler
notices inside each image. [Docker attestation documentation](https://docs.docker.com/build/ci/github-actions/attestations/)

It then packages the chart with the tag's version/appVersion and manager digest as its default image,
substitutes that digest into raw installation manifests and creates the GitHub release with image
references, LICENSE and SHA256SUMS.
No standalone binary matrix or custom signed inventory is distributed. GHCR uses the repository's scoped
`GITHUB_TOKEN`; no cloud account, KMS or manually managed release key is required.
[GitHub publishing documentation](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)

BuildKit provenance and checksums are build/integrity metadata, not independent publisher signatures.
For this private project, trust begins with access to the expected repository/registry, reviewed main
history and controlled tag writers; install by digest. The workflow grants package writes only to the
image jobs and release writes only to the packaging job. Protect main and version-tag creation in the
repository settings before enabling publishing. Keyless signatures can be added with standard tooling
if the distribution policy requires independent publisher verification; evaluate transparency disclosure
and private-repository feature availability first. No runtime process should own release-signing keys.

## Runtime and installation

Administrators provide a separately accepted runtime image for both orchestrator and session runners.
The optional [runtime candidate](../../images/runtime/README.md) has unresolved Critical findings and is
not release-approved. Its custom curl/OpenSSH/npm maintenance stays isolated and can be replaced by a
compatible accepted image without rebuilding the operator.

The environment credential is an existing namespace Secret. cert-manager handles admission TLS. New
Fleets start suspended; [network approval and drain](../operations/install-and-drain.md) remain runtime
requirements. Neither artifact publication nor a successful Pod smoke test proves registration, OAuth
renewal, session recovery, network enforcement or redistribution rights for vendor software.
