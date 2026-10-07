---
type: Repository Documentation
title: "Implementation evidence"
description: "Observed checks and explicit acceptance gaps."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T19:32:32+07:00"
---

# Implementation evidence

This ledger describes the implementation candidate, not production acceptance. Run commands from the feature
worktree. Implementation commit: `a26bed1d33aca0a645a3d8979dc057b316a4aaa0`.
The [GitHub verification run](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37575653587)
passes all jobs: unit/race/vet/dependency checks, generated-source parity, Linux AMD64/ARM64 cross-builds and both API-server matrix versions.
The clock correction at `938d9eb` passes local `make verify` and the expanded API-server suite on 1.33.0, 1.36.0 and 1.37.0. [Its CI run](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37580606346) passes all jobs, including both original API versions and cross-builds. [The expanded CI run at `66e7557`](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37582246718) passes unit/generation/cross-build checks and all three API versions.
The tested ARM64 manager architecture-manifest digest at recovery commit `79b8921` is
`sha256:71cdd7313c567406bd999f5eb9e1e409add3353f3c1480041e7534634bf454bb`.
This is a local private build, not a published release. The earlier CI link applies to its named commit;
follow the PR checks for the latest revision.

| Check | Observed evidence | Limits |
| --- | --- | --- |
| Unit/race and dependency checks | `go test -race ./...`, `go vet ./...`, `go mod verify` passed. | Pure builders, lifetime bounds, metrics and token transport; not a running cluster. |
| API-server recovery | The expanded suite passed under Kubernetes 1.33.0, 1.36.0 and 1.37.0 with the race detector in isolated Linux containers. | Envtest has no scheduler, garbage collector or CNI. |
| Receipt recovery | Concurrent callers, receipt/Secret response loss, mismatched credential and snapshot/status denial pass. A completed matching receipt is re-read before acknowledging a failed credential/completion write. | Deterministic completion-before-Secret-admission and lost-completion-response recovery pass; foreign Secret, credential mismatch and immutable intent denials remain enforced. |
| Launch recovery | Fence crash, late Pod, lost create response and two concurrent reconciles produce no repeated submission. | Admission dependency-read faults at Fleet, key, report, receipt and credential boundaries return sanitized retryable responses and recover. Physical API unavailability, scheduling exhaustion and worker stop/administrative decommission pass in the synthetic kind fixture. Real vendor recovery and production p99 remain pending. |
| Lifecycle | Suspension gates an unlaunched order; running Pods survive expiry; drain and explicit Abort survive controller replacement, preserve unrelated Pods and diagnostic retention. Abandoned partial intake cleans up without submission; expired completed redelivery does not recreate a Secret. | Pending startup tests cover never-started deletion, a fresh read observing Running, and a resourceVersion conflict when Running races with DELETE. Retention caps and projected service-account token refresh pass. Active vendor drain remains pending. |
| Sanitized observations | API tests verify receipts/events do not contain synthetic JWTs and Fleet terminal counts survive Pod removal; metrics unit tests exclude private UID labels. | Infrastructure observations only; vendor outcomes remain unknown. |
| Fleet convergence | Repeated unchanged reconciles preserve Deployment resourceVersion. | Kind 1.37 installed admission is ready; hook/session/manager namespace permission checks deny unauthorized access. |
| Native connection probe | Synthetic transport tests cover connected/disconnected/missing state, 401/503, transport/read errors, malformed/trailing/oversized JSON, body closure and redirect rejection. The request remains local and bounded to two seconds. | Synthetic revoked/missing-key rotation passes in kind; connection follows only the current polling generation. Does not prove native revoked-key behavior or actual vendor connection. Task 2.3 retains that acceptance gate. |
| Hook runtime | AMD64 and ARM64 images install the hook under a read-only root with UID 1000 within the configured 32Mi hook volume. Reproduce with `sh hack/hook-smoke.sh IMAGE PLATFORM`. | AMD64 ran through QEMU on an ARM64 host; it is not a native AMD64-host test. |
| Vendor runtime | AMD64 and ARM64 private images verify signing-key fingerprint, manifest signature and binary checksum. Read-only UID 1000 smoke prints Claude 2.1.285, Node 24.21.0, Python 3.11.2 and git 2.39.5; required flags exist. | Reproduce with `sh hack/runtime-smoke.sh IMAGE PLATFORM`. AMD64 ran through QEMU. Real registration, session turns and git push remain unverified. |
| Packaging | Helm lint and render pass; raw and Helm resources generated from Kustomize. | Helm and raw install/update/admission/suspended-Fleet drain/removal pass on kind 1.33.1, 1.36.4 and 1.37.0 with the clock-fix build. The loaded OCI index matches the requested digest. CRDs and administrator namespace remain; active vendor drain is pending. |
| Enforced network | Isolated kind 1.37.0 with Cilium 1.20.2 and cert-manager 1.21.2 passed testing; the cluster was removed. | Two fresh selected Pods deny direct public/private/API/metadata/Pod Identity probes with explicit Cilium policy-denied flows. Approved HTTPS through the proxy passes CA validation, private/IP CONNECT is rejected. This is not downstream approval. |

The reproducible fixture is [`hack/network-smoke.py`](../../hack/network-smoke.py); the sanitized result is
[Cilium smoke evidence](evidence/cilium-smoke.json). An unrestricted control reaches public/private/API targets.
The metadata control cannot connect, so explicit CNI drop events are required to substantiate that denial.
The fixture never creates an activation report and is not a production proxy.

## Installation findings captured by real testing

A Secret webhook matching all actors blocked its own cert-manager TLS bootstrap and Helm release recording.
The credential webhook now matches authenticated hook service accounts through API-server CEL conditions,
keeping their writes fail-closed while other authorized Secret administrators can initialize the installation.
This follows [Kubernetes request filtering](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#matching-requests-matchconditions), available since Kubernetes 1.30.

RBAC comments attached to functions were ignored by controller-gen, leaving a stale initial Role. Markers
now form package-level blocks and explicitly generate namespaced Roles. The installed smoke test verifies
permissions, not only generator exit status. The manager must register handlers through
`mgr.GetWebhookServer()` so controller-runtime adds the server to its runnable lifecycle.

## Clock translation recovery

Private hook builds from clock-fix source `938d9eb` passed `hack/hook-smoke.sh` on both platforms:

| Platform | OCI index digest | Architecture manifest digest |
| --- | --- | --- |
| linux/amd64 | `sha256:34ea7a982dd8ac0c569d3b36b748ef2be31032e17b207aaee1a4dd4d617dbb29` | `sha256:6be3efde5c4e5d533e0d9f0ea09a0f42028301427d71cf02f7136d46faac48b1` |
| linux/arm64 | `sha256:1e534a387c13ed3335d30fa4e0c14239667a4a4a096c2af8a91696abf4c0a284` | `sha256:f39911cc00687860ccf97a7cfa6cc4f250b8b1302b5a6ba570b46dca6869bff7` |

Docker image inspection matches these index digests. Both install the hook with UID 1000, a read-only
root, all capabilities dropped and no new privileges into the 32Mi hook volume. AMD64 uses QEMU.
These pins identify that source revision; subsequent hook changes require their own build evidence.

The native poll HTTP Date determines token lifetime when present. Signed `expiresAt` remains unchanged;
`clockOffsetSeconds` captures local-minus-server time once, bounded to ±3600 seconds. Admission and launch
use the corrected cluster deadline. Completion checks the actual credential against that same time basis.
An incomplete retry with a missing Date preserves the first offset. Schema and admission forbid changing it.
Terminal retention and unobserved in-flight credentials use the later of signed/corrected expiry plus margin.

`make verify` and the race/API suites on 1.33.0 and 1.37.0 pass. Pure tests exercise both one-hour bounds,
server-expired and excessive lifetimes, malformed Date, retryable excessive skew and immutable translation.
API scenarios with ±240 seconds verify incomplete repair, a later/missing Date, completion, one launch,
running preservation after expiry, terminal credential removal and replay without recreation. These are
synthetic lifetime tests, not native JWT signature or registration acceptance. Synchronize cluster node clocks;
the stored offset corrects vendor-to-cluster time and cannot compensate for independent node drift.

## Minimum-version installation and dependency limits

The separate Kubernetes 1.33.1 cluster used [kind 0.29.0's published node pin](https://github.com/kubernetes-sigs/kind/releases/tag/v0.29.0):
`kindest/node:v1.33.1@sha256:050072256b9a903bd914c0b2866828150cb229cea0efe5892e2b644d5dd3b34f`.
Cilium 1.20.2 and cert-manager 1.21.2 were Ready before installation. Both Helm and raw paths ran against
this cluster with Restricted namespace policy and an explicitly pinned manager. The raw path followed a
complete Helm uninstall; there were no overlapping installations. Its suspended example passed server dry-run,
converged and drained. Namespaced and cluster installation resources were then removed while retaining CRDs
and the administrator namespace. Each completed disposable cluster was removed after evidence capture; an unrelated pre-existing cluster was left intact.

[Cilium's compatibility guarantee](https://docs.cilium.io/en/stable/network/kubernetes/compatibility/) and
[cert-manager's tested version range](https://cert-manager.io/docs/releases/) currently cover Kubernetes 1.33–1.36
for these dependency releases. The 1.37 installation/network result is local empirical evidence, not their
upstream compatibility guarantee. Do not advertise a universal supported production matrix from these tests.

The reproducible packaging fixture is [`hack/install-smoke.py`](../../hack/install-smoke.py), restricted to an
explicit disposable operator kind context and fresh installation namespace. It verifies loaded image targets
against the requested digest, then runs Helm and raw install/update/admission/RBAC/suspended-drain/removal.
[Minimum-version evidence](evidence/packaging-v133.json), [dependency-compatible 1.36 evidence](evidence/packaging-v136.json) and [forward-version evidence](evidence/packaging-v137.json) records the tested OCI index digest. The initial local
child-digest alias pointed at the image index; it contained the correct architecture image, but we corrected
the pin and reran both paths. The fixture rejects that alias mismatch before installation. This is local
private-image evidence, not a signed release or active vendor drain.

The dependency-compatible cluster used [kind 0.33.0's published node pin](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0):
`kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`.
It passed the same default-namespace packaging fixture and loaded image check on both paths. This ledger
reports exact tested versions; it does not establish every intermediate minor/patch or production vendor compatibility.

## Requirement audit

| Plan requirement | Candidate evidence | Remaining acceptance |
| --- | --- | --- |
| Declarative fleet intent | CRD defaults, admission, suspended cluster example and scoped convergence. Labels/tolerations/proxy Service and reference names are rejected before submission; portable valid tolerations pass unit tests. | Typed label/toleration/proxy/reference rejection now passes real admission tests; downstream configuration remains pending. |
| Supported native intake | Native CLI flags, durable repair/concurrency tests, permanent/transient API classifications. | Clock translation is immutable and checked through completion, launch, retention and missing-Date partial retry. Real native dispatch remains unverified. |
| No repeated operator submission | Fence/write-boundary recovery, concurrent controllers, single POST transport and lost/late Pod tests. | The 20-order physical synthetic fixture passes faults and replay; production throughput and native registration p99 remain pending. |
| Session isolation | Pod builders, installed RBAC denial and enforced Cilium/proxy tests from two fresh Pods. | Downstream network approval and vendor session execution. |
| Retention and credential safety | Terminal replay, incomplete intake, retention caps, pending/Running DELETE races and fresh projected token transport. | Positive/negative offsets preserve the later of signed and corrected expiry plus margin; running Pods survive that floor. Stable synchronized cluster clocks remain an operating prerequisite. |
| Safe suspension and deletion | Suspend, scoped Drain/Abort, waiting generation, replacement controller and suspended installed-cluster removal. | Active vendor drain and downstream rollback rehearsal. |
| Honest observations and verification | Sanitized status/events/metrics distinguish infrastructure and connection. | Dedicated registration/turn/git tests, reviewed support matrix, signed release and spec approval. |

This audit does not sync draft delta specifications into accepted current specifications.

## Concurrent batch and acknowledgement recovery

`make verify` and the updated API/race suite pass on 1.33.0, 1.36.0 and 1.37.0. The batch exercises 32
distinct orders with four concurrent deliveries each, bounded to eight test API callers. Two competing
reconciles per order face a lost Pod-create response; a replacement controller observes every Pod UID.
Each order records one Pod POST and one durable receipt. Pod deletion followed by another reconcile
does not issue another POST. This demonstrates no two-session controller cap within that synthetic batch.

The batch exposed a concurrency fault: credential admission can see a completed receipt before another
delivery's credential request reaches storage, producing a denial rather than AlreadyExists. The hook
now re-reads after credential/completion write failure and acknowledges only the same UID, matching
immutable snapshot, valid Fleet owner, completed state and no deletion timestamp. The credential webhook
still denies that write. A deterministic API test completes the winning delivery immediately before the
loser's Secret admission; the loser returns the durable accepted result. A different credential remains
permanently rejected. Lost completion responses can also be acknowledged after confirming persistence.

Envtest does not execute these Pods or run a scheduler/kubelet/CNI. These are bounded API concurrency and
recovery checks, not production throughput, startup p99, node loss or scheduling-capacity acceptance.
Task 6.4 remains pending for native registration startup p99 and production capacity tuning; the physical synthetic slice is recorded below.

The hook was rebuilt from an archived, clean source commit `8ff44dc70cae413761d911c6bac0ca76b9a044b9`,
including the readiness and acknowledgement fixes. Both private images pass `hack/hook-smoke.sh` under
the configured read-only/non-root/32Mi volume posture; AMD64 executes through QEMU on the ARM64 host.

| Platform | OCI index digest | Architecture manifest digest |
| --- | --- | --- |
| linux/amd64 | `sha256:97c12e3a80fb2aa832399a4156303485e2ea3dae0046aca2b91d77a916479c19` | `sha256:aa030e771ebdb97121b3c758ba83b08bebc2d5030e994747226bb205df5b8be8` |
| linux/arm64 | `sha256:a66e4fc8bec59077b5f5f697c7946125fbc212a2b9a2d1c994f9dfdf8e19aa47` | `sha256:9bf2b8f2cdef3ef73cb4f0f8fe2ede6a02f8f299f9f9091f3625915361ecd950` |

Docker image inspection matches these index digests. They are local private builds; they do not replace
the source-specific clock-fix manager installation evidence or constitute published/signed artifacts.

## Physical synthetic faults and polling revisions

The [`hack/fault-smoke.py` fixture](../../hack/fault-smoke.py) passed on a fresh two-node kind 1.36.4
cluster with cert-manager 1.21.2. [Sanitized evidence](evidence/fault-smoke.json) records exact local image
pins, Pod UIDs, checks and 20 orders with one completed, successful audited Pod-create request each.
Metadata-only audit captured no request/response bodies. Loaded OCI targets match the requested digests.
The fixture created the cluster and removed it after capture; the unrelated pre-existing cluster was untouched.

Sixteen distinct synthetic runners ran and redelivery retained their UIDs. Revoked/missing synthetic keys
degraded polling and restoration replaced its process without changing an accepted runner. Worker cordon
produced an unschedulable pending order; expiry plus the full margin reclaimed it and its credential, and
terminal replay recreated neither. Uncordon allowed a fresh order. Pausing the control plane made its API
unavailable; the hook returned a sanitized retryable result, existing runner UIDs survived, and the same
unaccepted order ran once after recovery. Worker stop first produced Ready=Unknown. Explicit administrator
Node removal then triggered orphan cleanup; a new Node UID and fresh order recovered, while replay of the
lost order created no replacement. Abort removed owned execution while the replay-retention finalizer stayed.

The initial physical key test exposed a default rolling-update bug: an old available poller kept the Fleet
Connected while its new bad-key revision was unready. Orchestrator upgrades now use `Recreate`, and connection
requires the current observed generation, updated/total replica agreement and current availability. This
introduces a brief polling outage during template upgrades; accepted session Pods remain independent.
The new API regression also checks conversion of an existing RollingUpdate child and stale-generation status.
`make verify` and the full API/race suites on 1.33.0, 1.36.0 and 1.37.0 pass with this polling fix;
the API matrix was rerun sequentially after removing the disposable fault cluster.

The tested manager OCI index is `sha256:744729f1339e2fe53e234d184be936971e51553b0a7890995b466c5ab73fe095`.
It was built in the worktree based on `59d970e` with this PR's Recreate/current-generation production delta;
`sourceBaseCommit` denotes that base, not a claim that the base alone includes the fix. The synthetic runtime
index is `sha256:149be6b5feeaf2e88956991c328fb8ddf51ebf46fc5e3db5147451f8a5f70f08`; its Go source SHA-256 is
recorded in the result. The hook is the previously verified `8ff44dc` ARM64 build. These are local private images.

The labelled runtime never contacted Anthropic or registered a session. Its activation report is explicitly
a synthetic fixture assertion, and kind's default CNI does not prove enforcement. The worker was
administratively decommissioned; this does not certify automatic cloud-node recovery or downstream eviction
policy. No production throughput or native registration startup p99 was measured. Tasks 2.3/6.3/6.4 retain
their corresponding real integration and tuning gates.

## Historical private candidate and vulnerability checks

The offline bundle/signature tools described in this section were retired by the simplicity review.
These records describe their pinned source revisions, not the current release path. The notice collector
is retained and now includes upstream notices in manager/hook images. See [operator releases](../workflows/release-candidates.md).

Source commit `aa0318de15628e9e8c6fe729fd9c5cdf1caa66cd` builds a clean private review bundle with
`hack/build-candidate.py`. The [sanitized result](evidence/candidate-smoke.json) records the source/tree
and candidate manifest digests, Go 1.27.1, Syft 1.54.1, govulncheck v1.8.0 and Cosign 3.1.3.
The 18 inventoried files include four Linux AMD64/ARM64 binaries, four CycloneDX SBOMs, build information,
architecture-specific scan output, raw installation resources, the chart and source license. All 63 compiled
manager modules and 55 compiled hook modules appear in each architecture's SBOM. The chart archive passes
Helm lint with a verified manager digest; an image remains required at installation.

Source scans report no known reachable Go vulnerabilities for Linux AMD64 and ARM64. A separate disposable
analysis-only fixture calls `language.ParseAcceptLanguage` from `golang.org/x/text@v0.3.7`; the scanner
reports [GO-2022-1059](https://pkg.go.dev/vuln/GO-2022-1059) and returns a failing exit code. The fixture is
not executed and does not modify production dependencies. Text mode gates
on findings/errors; JSON/SARIF output would not supply that exit-code gate. The old scanner release's analysis
library did not understand Go 1.27 syntax. The verified module tag v1.8.0 and correctly selected compiler
provide the tested analysis. This does not clear container OS/native-binary vulnerabilities or future advisories.

The default producer rejects dirty source before creating output. Six independent inventory tests pass.
The real Cosign exercise uses ephemeral test-only keys and passes signature/inventory verification plus
wrong-key, changed-manifest, same-size binary tamper, missing/extra file, symlink and candidate-provided-key
rejection. The original candidate is not signed by that fixture; its private test copy and keys are removed.
The result explicitly records no production signature, vendor runtime or publication.

`make verify`, `make test-api` on 1.33.0, actionlint, wiki validation and redacted secret scan pass for this
preparation. The former candidate guide recorded verified tool asset pins, build/sign/verify reproduction and the
private key trust policy at this source revision; the [release guide](../workflows/release-candidates.md) now describes the replacement path. The policy has no public transparency log,
certificate identity or timestamp proof; production signing identity, key lifecycle, license review, container
artifact checks and release approval remain pending. Task 7.3 is not marked complete from this exercise.

## Compiled dependency notices

Clean source `1641e62cf89ea0d017a0b1d28da29f0acfa2ff4f` produces a 20-file review bundle through
the candidate producer, with [sanitized notice/signature evidence](evidence/notices-smoke.json).
The added archive/index bind 102 original notice files from 63 compiled modules and 38 from the selected
Go 1.27.1 toolchain. Independent reads of all four binaries' embedded build information match the stored
module/version/checksum records and binary bindings: 63 manager and 55 hook modules per architecture.
Every one of the 140 archived files matches its source bytes, size and hash; the archive has normalized
metadata and reproduces identically from the same sources. No local cache paths enter the index.

The collector runs module-cache integrity verification before and after reading, and rejects a compiled
module outside the current unreplaced build list before collection. This boundary matters because
`go mod verify` covers the build list, not arbitrary downloaded versions. Ten inventory/notice tests pass,
covering nested/variant license and patent files, original bytes, deterministic archives, missing license,
symlinks, inconsistent module/compiler identity, replacement provenance, unsafe paths and size limits.
`make verify` and the real 1.33.0 API suite pass after the collector change; actionlint, wiki/link validation,
diff whitespace and redacted secret scan pass.

Actual Cosign signing with temporary test-only keys verifies this clean candidate and rejects modified
notice archive/index bytes in addition to the previous binary, manifest, missing/extra, symlink and trust-key
failures. The original candidate remains unsigned, and the temporary signing copies/keys are removed.
The notice scope deliberately includes whole module/toolchain trees, including uncompiled code; filename
discovery does not analyze embedded source headers or interpret license obligations. The index explicitly
retains `licenseReviewApproved: false` and excludes the vendor runtime. Tasks 1.2/7.3 remain pending for
licensing, container artifacts, vendor/public naming, signing trust, support and release approval.

## Container scans and updated SDK

The [runtime review](runtime-vulnerability-review.md) and [sanitized image scan evidence](evidence/container-scan.json)
record actual Grype 0.120.1 scans without suppression. The prior ARM64 manager/hook candidates have no matches.
Both updated runtime architectures retain 27 Critical matches covering 10 advisory IDs. A checksum-pinned npm
11.21.0 update removes four High/five Medium matches, but three fixable High bundled-dependency matches remain.
Native ARM64 read-only/offline toolchain smoke passes; the AMD64 build and scan pass, while local QEMU execution
fails. [Native AMD64 CI](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37602263943) now passes
the same full toolchain/CLI flag/offline npm smoke as native ARM64. Task 5.1 is restored after changed-image verification;
no vulnerability waiver, production signature, vendor session or release publication is claimed.

`make verify` and the real 1.33.0 API suite pass after the SDK/smoke/CI change. Successful code tests do not clear
container Critical findings; task 7.3 remains pending.

## npm bundled dependency overrides

The [runtime review](runtime-vulnerability-review.md) and [override evidence](evidence/npm-overrides-smoke.json)
record compatible, checksum-pinned upstream brace-expansion 5.0.11/undici 6.28.1 replacements with retained
license files and an image-local override record. Both final local architecture scans remove all three
previously fixable bundled-dependency High matches without suppression; remaining counts include 27 Critical
and 126 High matches. The Critical release gate still fails. ARM64 native SDK and actual advisory probes pass;
the old bundle reproduces the reported failures. [Matching AMD64 hardware CI](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37604394799) passes for this changed image, including actual advisory and offline SDK probes.
Task 5.1 is restored after that execution check; no signing key is generated or release published.

## Debian 13 and Python runtime revision

The [runtime review](runtime-vulnerability-review.md) and [new candidate evidence](evidence/trixie-runtime-smoke.json)
record digest-pinned official Python 3.14.8/Node 24.21.0 images on one Debian 13 suite with a fixed apt snapshot.
Node's SDK/npm/npx/Yarn and Python's runtime/venv/pip are retained. The named UID/GID 1000 runner account
supports SSH user lookup. Python changes from 3.11 to 3.14, so application dependency compatibility remains
an acceptance concern. Native ARM64 restricted smoke passes the CLI/npm advisory checks and offline SDK
operations plus CA/SSL, compression, ctypes, SQLite and virtual-environment wheel install/import.

Both final local architecture scans report 25 Critical, 100 High, 74 Medium, 13 Low, 103 Negligible and
one Unknown match. Eight prior Python/glibc/SQLite Critical package matches disappear; six curl matches
are added for two newer-version advisory ranges. No user ignore rules or ignored matches are present;
the effective Grype configuration retains its default kernel-header exclusions. The Critical gate still fails.
A separately hashed Syft catalogue identifies the upstream Python binary and actual package versions.
`make verify` and the 1.33.0 API suite pass. [Matching native AMD64 CI passes](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37607651747/job/112746981191)
for source 4ed9067, including Python venv/offline pip and all prior CLI/npm probes. Task 5.1 is restored
for this revision. No source tests or smaller scanner count substitute for release acceptance.

## Upstream SSH client revision

The [runtime review](runtime-vulnerability-review.md) and [source-bound evidence](evidence/openssh-runtime-smoke.json)
record pinned upstream OpenSSH 10.6p1 source/signature verification, client-only installation and original
license-byte retention. Native ARM64 full restricted SDK smoke passes SSH signing/tamper rejection,
agent sign/remove and local scp. GSSAPI authentication is retained; Debian's GSSAPI key-exchange extension
is absent and explicitly rejected. Actual remote SSH, vendor Git and hardware tokens remain untested.

Grype reports 24 Critical matches, all in curl/libcurl, with zero ignored matches and a failed release
threshold. Syft does not identify the compiled SSH clients; the image's pinned source/binary build record
supplements that inventory gap. The missing old package match alone does not prove a security fix.
Local AMD64 compilation failed under QEMU; [matching native AMD64 CI passes](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37611163966/job/112758476250)
for exact source f261cc1, including all SSH/SDK checks. Task 5.1 is restored; the new image has only
an ARM64 container rescan, and no AMD64 scan clearance is claimed.
Local `make verify` and the real Kubernetes 1.33.0 API-server suite pass. No release or vendor acceptance
is inferred from these results.

## curl/libcurl and native scan revision

The [runtime review](runtime-vulnerability-review.md) and [candidate evidence](evidence/curl-runtime-smoke.json)
record signature/checksum-verified curl 8.22.0, private versioned packages, original license bytes and
preserved OpenSSL/GnuTLS exported ABI symbols. Debian's GnuTLS namespace/SONAME are explicitly retained.
The GnuTLS CLI preserves HTTP/3; upstream protocol removals and TLS-backend changes remain compatibility
concerns. Both old variants reproduce proxy-credential clearing/netrc user leakage, and both new variants
pass the same real API probes with positive controls. Native ARM64 restricted SDK smoke passes actual
CLI/library CA trust/rejection and Git HTTPS clone/commit/push in an offline disposable container fixture.
No real vendor Git or Kerberos identity switching is tested. [Matching native AMD64 SDK smoke passes](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37614758051/job/112770303228).
The tested PR merge has `033c4e2` as a parent and matches all 13 recorded build/smoke/scan source hashes.

Syft identifies all three custom 8.22.0-operator1 packages, but Grype's Debian `wont-fix` records still
produce 24 Critical matches with zero ignored matches and exit 2. There is no automatic VEX/waiver or
ambient-user advisory resolution. CI now downloads pinned scanner tools, enforces that threshold and
retains only private JSON review metadata, not images/binaries. SDK execution and scan outcomes remain
separate evidence; task 5.1 is restored and task 7.3 stays pending. The native AMD64 scan also
reports 24 Critical, 98 High, 68 Medium, 10 Low, 95 Negligible and one Unknown, zero ignored and exit 2.
Private JSON upload succeeds after that failure. Its exact source/image/tool/database/log/report/SBOM
bindings are retained in the sanitized evidence; the overall runtime job is failed.

The [extended curl regression evidence](evidence/curl-boundary-regression.json) adds identical
old/new comparisons for Digest origin switching, environment-selected proxy switching, PSL trailing-dot
cookies and CLI netrc user isolation. Eight disclosure failures reproduce on the old runtime; the current
runtime passes valid authentication/cookie controls and full restricted native ARM64 SDK smoke.
These extend behavior evidence to five advisory IDs; matching extended AMD64 execution remains pending.
The image itself is unchanged and no scan exception or release clearance is introduced.

## External gates

Dedicated test-only environment credentials and OAuth registration are not available in this worktree.
Real vendor acceptance, enforced downstream egress, measured downstream startup p99, non-overlapping migration,
reviewed release signatures and vendor redistribution remain incomplete. No live fleet is activated and no
release is published. Session success is never inferred from Pod exit status.

## Simplicity revision verification

The revision separates operator release CI from the optional runtime, removes the offline bundle/signature
path and makes network-report expiry optional. Local `make verify` passes (unit/race, vet, dependency
integrity, generation/packaging and four notice tests); all eight remaining Python tests pass when the
separate runtime archive tests are included. Real API-server suites pass on Kubernetes 1.33.0 and 1.37.0,
including unchanged approval after 30 days, explicit expiry, missing/failed/mismatched approval, revoked
polling and blocked unlaunched orders. Existing crash, concurrency, retention and replay cases still pass.

Manager/hook images build for Linux AMD64 and ARM64. Native ARM64 and emulated AMD64 hook installation
passes with a non-root UID, read-only root and the 32Mi hook volume. Both images include 140 original
module/toolchain notice files. The manager's restricted offline help path runs. The exact release tag
guard accepts valid stable/prerelease tags and rejects malformed tags, leading-zero numeric prereleases
and commits outside main history. The exact packaging step passes chart lint/package with version/appVersion,
substitutes an actual local manager digest into raw installation and verifies all SHA256SUMS. Actionlint,
workflow YAML parsing and local Markdown/wiki checks pass.

Redacted Gitleaks returns five generic-key matches in unchanged runtime/evidence files: one public
release-key archive hash, one test-log hash, one public signing-key fingerprint and two binary hashes.
No new match or suppression is introduced. This is reviewed scanner output, not a zero-findings claim.
The manager's multi-platform OCI export also contains an SPDX SBOM and SLSA provenance for each
architecture using the release build flags. Registry publishing, tagged-release attachment and real
Claude/network acceptance have not been executed by this local verification. Remote source/image CI
is checked separately on the pushed revision.
