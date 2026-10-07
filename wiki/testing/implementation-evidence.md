---
type: Repository Documentation
title: "Implementation evidence"
description: "Observed checks and explicit acceptance gaps."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T13:02:00+07:00"
---

# Implementation evidence

This ledger describes the implementation candidate, not production acceptance. Run commands from the feature
worktree. Implementation commit: `a26bed1d33aca0a645a3d8979dc057b316a4aaa0`.
The [GitHub verification run](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37575653587)
passes all jobs: unit/race/vet/dependency checks, generated-source parity, Linux AMD64/ARM64 cross-builds and both API-server matrix versions.
The subsequent recovery changes pass local `make verify` and the expanded API-server suite on both versions.
The tested ARM64 manager architecture-manifest digest for these changes is
`sha256:71cdd7313c567406bd999f5eb9e1e409add3353f3c1480041e7534634bf454bb`.
This is a local private build, not a published release. The earlier CI link applies to its named commit;
follow the PR checks for the latest revision.

| Check | Observed evidence | Limits |
| --- | --- | --- |
| Unit/race and dependency checks | `go test -race ./...`, `go vet ./...`, `go mod verify` passed. | Pure builders, lifetime bounds, metrics and token transport; not a running cluster. |
| API-server recovery | The expanded suite passed under Kubernetes 1.33.0 and 1.37.0 with the race detector in isolated Linux containers. | Envtest has no scheduler, garbage collector or CNI. |
| Receipt recovery | Concurrent callers, receipt/Secret response loss, mismatched credential and snapshot/status denial pass. | Includes foreign Secret rejection, Fleet UID/expiry forgery and hook/admin status, deletion and snapshot denials; persisted fence/Pod/terminal status response loss. |
| Launch recovery | Fence crash, late Pod, lost create response and two concurrent reconciles produce no repeated submission. | Admission dependency-read faults at Fleet, key, report, receipt and credential boundaries return sanitized retryable responses and recover. Physical API unavailability, node loss and scheduling exhaustion remain pending. |
| Lifecycle | Suspension gates an unlaunched order; running Pods survive expiry; drain and explicit Abort survive controller replacement, preserve unrelated Pods and diagnostic retention. Abandoned partial intake cleans up without submission; expired completed redelivery does not recreate a Secret. | Pending startup tests cover never-started deletion, a fresh read observing Running, and a resourceVersion conflict when Running races with DELETE. Retention caps and projected service-account token refresh pass. Active vendor drain remains pending. |
| Sanitized observations | API tests verify receipts/events do not contain synthetic JWTs and Fleet terminal counts survive Pod removal; metrics unit tests exclude private UID labels. | Infrastructure observations only; vendor outcomes remain unknown. |
| Fleet convergence | Repeated unchanged reconciles preserve Deployment resourceVersion. | Kind 1.37 installed admission is ready; hook/session/manager namespace permission checks deny unauthorized access. |
| Hook runtime | AMD64 and ARM64 images install the hook under a read-only root with UID 1000 within the configured 32Mi hook volume. Reproduce with `sh hack/hook-smoke.sh IMAGE PLATFORM`. | AMD64 ran through QEMU on an ARM64 host; it is not a native AMD64-host test. |
| Vendor runtime | AMD64 and ARM64 private images verify signing-key fingerprint, manifest signature and binary checksum. Read-only UID 1000 smoke prints Claude 2.1.285, Node 24.21.0, Python 3.11.2 and git 2.39.5; required flags exist. | Reproduce with `sh hack/runtime-smoke.sh IMAGE PLATFORM`. AMD64 ran through QEMU. Real registration, session turns and git push remain unverified. |
| Packaging | Helm lint and render pass; raw and Helm resources generated from Kustomize. | Helm install/upgrade and suspended-Fleet drain/uninstall pass on kind 1.33.1 and 1.37.0. Raw install/admission/drain/removal pass separately on 1.33.1. CRDs and administrator namespace remain. Upper-version raw lifecycle and active vendor drain remain pending. |
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

## Minimum-version installation and dependency limits

The separate Kubernetes 1.33.1 cluster used [kind 0.29.0's published node pin](https://github.com/kubernetes-sigs/kind/releases/tag/v0.29.0):
`kindest/node:v1.33.1@sha256:050072256b9a903bd914c0b2866828150cb229cea0efe5892e2b644d5dd3b34f`.
Cilium 1.20.2 and cert-manager 1.21.2 were Ready before installation. Both Helm and raw paths ran against
this cluster with Restricted namespace policy and an explicitly pinned manager. The raw path followed a
complete Helm uninstall; there were no overlapping installations. Its suspended example passed server dry-run,
converged and drained. Namespaced and cluster installation resources were then removed while retaining CRDs
and the administrator namespace. The isolated cluster was removed; an unrelated pre-existing cluster was left intact.

[Cilium's compatibility guarantee](https://docs.cilium.io/en/stable/network/kubernetes/compatibility/) and
[cert-manager's tested version range](https://cert-manager.io/docs/releases/) currently cover Kubernetes 1.33–1.36
for these dependency releases. The 1.37 installation/network result is local empirical evidence, not their
upstream compatibility guarantee. Do not advertise a universal supported production matrix from these tests.

## Requirement audit

| Plan requirement | Candidate evidence | Remaining acceptance |
| --- | --- | --- |
| Declarative fleet intent | CRD defaults, admission, suspended cluster example and scoped convergence. | Complete typed placement validation audit and downstream configuration. |
| Supported native intake | Native CLI flags, durable repair/concurrency tests, permanent/transient API classifications. | Server-time/clock-skew handling is inconsistent across hook, admission and controller; reopened task 3.1. Real native dispatch unverified. |
| No repeated operator submission | Fence/write-boundary recovery, concurrent controllers, single POST transport and lost/late Pod tests. | Physical API/node/scheduling faults and scale remain pending. |
| Session isolation | Pod builders, installed RBAC denial and enforced Cilium/proxy tests from two fresh Pods. | Downstream network approval and vendor session execution. |
| Retention and credential safety | Terminal replay, incomplete intake, retention caps, pending/Running DELETE races and fresh projected token transport. | Clock-skew correction must preserve conservative expiry/retention floors. |
| Safe suspension and deletion | Suspend, scoped Drain/Abort, waiting generation, replacement controller and suspended installed-cluster removal. | Active vendor drain and downstream rollback rehearsal. |
| Honest observations and verification | Sanitized status/events/metrics distinguish infrastructure and connection. | Dedicated registration/turn/git tests, reviewed support matrix, signed release and spec approval. |

This audit does not sync draft delta specifications into accepted current specifications.

## External gates

Dedicated test-only environment credentials and OAuth registration are not available in this worktree.
Real vendor acceptance, enforced downstream egress, measured downstream startup p99, non-overlapping migration,
reviewed release signatures and vendor redistribution remain incomplete. No live fleet is activated and no
release is published. Session success is never inferred from Pod exit status.
