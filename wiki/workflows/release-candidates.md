---
type: Repository Documentation
title: "Operator releases"
description: "Standard image and chart publishing with separate runtime acceptance."
tags: [claude-code, release, supply-chain]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-08T16:30:00+07:00"
sources:
  - resource: repo://.github/workflows/release.yml
  - resource: repo://.github/workflows/test.yml
  - resource: repo://.github/workflows/runtime.yml
  - resource: https://docs.docker.com/build/ci/github-actions/attestations/
  - resource: https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images
---

# Operator images and chart

The public release unit is the manager image, spawn-hook image, OCI chart and raw installation/removal manifests. Claude's
native binary and the optional development runtime are excluded. The former offline review-bundle producer,
custom inventory verifier and signing exercise have been removed; their evidence remains historical.
Publication is demonstrated by the tagged workflow and its anonymous pull checks, not by source CI alone.
The dedicated native trial passed; target-installation acceptance remains required before production use of a Fleet.

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
notices inside each image. Both published platform images are scanned through the recorded release digest
using the existing Critical severity gate, rather than treating the separate CI build as the released artifact.
The scan reports are attached to the release. [Docker attestation documentation](https://docs.docker.com/build/ci/github-actions/attestations/)

It then packages the chart with the tag's version/appVersion and manager digest as its default image,
substitutes that digest into raw installation manifests and pushes the chart to
`oci://ghcr.io/OWNER/charts/claude-code-cloud-operator`. The chart version omits the source tag's `v`.
It records the chart digest/source commit in `chart.json`, checks that a registry pull yields the same
packaged archive, and hands the chart/manifests/image records/scans to a final release job.

That final job uses empty Docker/Helm credential configuration to pull the chart and both image platforms
anonymously. Only after those checks pass does it create the GitHub Release with all artifacts, LICENSE
and SHA256SUMS. An already released version is rejected; change the version for subsequent releases.
No standalone binary matrix or custom signed inventory is distributed. GHCR uses the repository's scoped
`GITHUB_TOKEN`; no cloud account, KMS or manually managed release key is required.
[GitHub publishing documentation](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)

BuildKit provenance and checksums are build/integrity metadata, not independent publisher signatures.
For this public project, trust begins with the expected repository/registry, reviewed main
history and controlled tag writers; install by digest. The workflow grants package writes only to the
image/packaging jobs and release writes only to the final release job. Protect main and version-tag creation in the
repository settings before enabling publishing. Keyless signatures can be added with standard tooling
if the distribution policy requires independent publisher verification; evaluate transparency disclosure
and feature availability first. No runtime process should own release-signing keys.

### First public release

Review tracked source/history for credentials and private deployment data before making the source public.
New personal GHCR packages default to private. Set the manager, hook and chart packages public in their
GitHub package settings and retain workflow access to the source repository. A public source repository
alone does not establish package visibility. If the final anonymous job fails while first-time visibility
is being configured, change visibility and rerun only the failed job; it reuses the existing artifact
without rebuilding or repushing images/chart. Do not recreate the tag or overwrite a published release.
[GitHub package visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)

After the first RC, test that released chart through dedicated Argo CD installation, update and retention
checks. Keep evidence tied to its source commit/chart/image digests. A public RC is not production approval
for every cluster; each consumer still validates its CNI, proxy, runtime and credentials.

## Runtime and installation

Administrators provide a separately accepted runtime image for both orchestrator and session runners.
The optional [runtime candidate](../../images/runtime/README.md) uses a maintained Ubuntu 26.04 devcontainer base and distro curl/OpenSSH,
with two temporary upstream npm dependency overrides. Native ARM64 security probes pass; runtime
acceptance still requires reviewed inventories, native AMD64 and real Claude/network acceptance.
A compatible accepted image can be supplied without rebuilding the operator.

Use the [public Helm/Argo guide](../operations/public-installation.md) and generic examples; consumer
cluster names, secret stores and team metadata stay downstream. The initial chart has one installation
per cluster, watches its namespace only, protects CRDs from Argo prune/deletion, and leaves Namespace
ownership to the administrator. The release's `uninstall.yaml` omits Namespace/CRDs and is used only after drain.

The environment credential is an existing namespace Secret. cert-manager handles admission TLS. New
Fleets start suspended; [network approval and drain](../operations/install-and-drain.md) remain runtime
requirements. Neither artifact publication nor a successful Pod smoke test proves registration, OAuth
renewal, session recovery, network enforcement or redistribution rights for vendor software.
