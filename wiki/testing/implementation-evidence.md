---
type: Repository Documentation
title: "Implementation evidence"
description: "Observed checks and explicit acceptance gaps."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T14:17:12+07:00"
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
| Launch recovery | Fence crash, late Pod, lost create response and two concurrent reconciles produce no repeated submission. | Admission dependency-read faults at Fleet, key, report, receipt and credential boundaries return sanitized retryable responses and recover. Physical API unavailability, node loss and scheduling exhaustion remain pending. |
| Lifecycle | Suspension gates an unlaunched order; running Pods survive expiry; drain and explicit Abort survive controller replacement, preserve unrelated Pods and diagnostic retention. Abandoned partial intake cleans up without submission; expired completed redelivery does not recreate a Secret. | Pending startup tests cover never-started deletion, a fresh read observing Running, and a resourceVersion conflict when Running races with DELETE. Retention caps and projected service-account token refresh pass. Active vendor drain remains pending. |
| Sanitized observations | API tests verify receipts/events do not contain synthetic JWTs and Fleet terminal counts survive Pod removal; metrics unit tests exclude private UID labels. | Infrastructure observations only; vendor outcomes remain unknown. |
| Fleet convergence | Repeated unchanged reconciles preserve Deployment resourceVersion. | Kind 1.37 installed admission is ready; hook/session/manager namespace permission checks deny unauthorized access. |
| Native connection probe | Synthetic transport tests cover connected/disconnected/missing state, 401/503, transport/read errors, malformed/trailing/oversized JSON, body closure and redirect rejection. The request remains local and bounded to two seconds. | Does not prove native revoked-key behavior or actual vendor connection. Task 2.3 retains that acceptance gate. |
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
| No repeated operator submission | Fence/write-boundary recovery, concurrent controllers, single POST transport and lost/late Pod tests. | Physical API/node/scheduling faults and scale remain pending. |
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
Task 6.4 remains pending for those criteria.

The hook was rebuilt from an archived, clean source commit `8ff44dc70cae413761d911c6bac0ca76b9a044b9`,
including the readiness and acknowledgement fixes. Both private images pass `hack/hook-smoke.sh` under
the configured read-only/non-root/32Mi volume posture; AMD64 executes through QEMU on the ARM64 host.

| Platform | OCI index digest | Architecture manifest digest |
| --- | --- | --- |
| linux/amd64 | `sha256:97c12e3a80fb2aa832399a4156303485e2ea3dae0046aca2b91d77a916479c19` | `sha256:aa030e771ebdb97121b3c758ba83b08bebc2d5030e994747226bb205df5b8be8` |
| linux/arm64 | `sha256:a66e4fc8bec59077b5f5f697c7946125fbc212a2b9a2d1c994f9dfdf8e19aa47` | `sha256:9bf2b8f2cdef3ef73cb4f0f8fe2ede6a02f8f299f9f9091f3625915361ecd950` |

Docker image inspection matches these index digests. They are local private builds; they do not replace
the source-specific clock-fix manager installation evidence or constitute published/signed artifacts.

## External gates

Dedicated test-only environment credentials and OAuth registration are not available in this worktree.
Real vendor acceptance, enforced downstream egress, measured downstream startup p99, non-overlapping migration,
reviewed release signatures and vendor redistribution remain incomplete. No live fleet is activated and no
release is published. Session success is never inferred from Pod exit status.
