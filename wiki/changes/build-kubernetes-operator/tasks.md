---
type: Implementation Plan
title: "Operator implementation plan"
description: "Ordered tasks with observable completion and release gates."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T18:04:52+07:00"
---

# Implementation tasks

Tasks are checked only when their complete verification passes. External release gates stay pending.

## 1. Contract and project foundation

- [x] 1.1 Evaluate the existing operator against crash/replay/security requirements; verification: record upstream gaps and select adoption, licensed fork or independent implementation before scaffolding.

- [ ] 1.2 Choose license and supported Go/Kubebuilder/Kubernetes/Claude versions; verify compatibility, binary integrity and redistribution policy before selecting release pins.
- [x] 1.3 Scaffold Go manager and hook commands; verification: build and unit test on AMD64/ARM64, dependency lock and credential-free CI.
- [x] 1.4 Add CRD types, defaults, status subresources and admission; verification: generated schema, rejected unsafe/cross-namespace/mutable-order inputs and gated intake until admission is ready.

## 2. Fleet reconciliation

- [x] 2.1 Reconcile namespace-scoped Deployment/config/RBAC/service accounts and pool claim; verification: repeat reconciles converge without touching unrelated resources.
- [x] 2.2 Reconcile suspended defaults and protected network report validation; verification: missing/stale/mismatched reports block activation and new launches.
- [ ] 2.3 Implement native connection status and credential-revision rollout; verification: revoked/missing credentials degrade status, rotation restarts polling without altering accepted runners.

## 3. Durable intake

- [x] 3.1 Implement adapter input checks and sanitized receipt naming; verification: missing/expired/malformed inputs and permanent/transient error classifications.
- [x] 3.2 Persist incomplete CR, immutable owned Secret and accepted snapshot; verification: concurrent redelivery, response loss, ownership collisions, partial-write repair and no JWT leakage.
- [x] 3.3 Restrict actor roles and one-time receipt completion through admission; verification: untrusted updates, Fleet UID mismatch and snapshot changes denied.

## 4. Work-order lifecycle

- [x] 4.1 Implement CAS launch fence and single Pod-create path with transport retries disabled; verification: two controllers and crashes at every write boundary never resubmit an order.
- [x] 4.2 Observe late/missing/terminal Pods and immutable UIDs; verification: ambiguous API outcomes and forced Pod deletion produce no replacement Pod.
- [x] 4.3 Implement credential removal, tombstone floors, capped retention and conservative pending cleanup; verification: replay after cleanup, expired orders, running/startup races and projected token rotation.
- [x] 4.4 Implement suspend, drain/abort deletion and finalizers; verification: scoped cleanup, waiting reasons, controller restart and uninstall instructions.

## 5. Runtime and security

- [x] 5.1 Build immutable non-root runtime and hook images with pinned native CLI; verification: verified downloads, toolchain/version tests, read-only smoke and updater disabled.
- [x] 5.2 Generate Pod security/storage/resources and configurable placement; verification: no API/environment key/cloud identity access from sessions and no fleet concurrency cap introduced.
- [x] 5.3 Establish proxy/egress integration and network acceptance report format; verification: direct/proxied denial and fresh-Pod startup tests on enforced-network clusters, not envtest alone.

## 6. Operations and delivery

- [x] 6.1 Add status/events/metrics without sensitive values or high-cardinality user labels; verification: redaction tests and terminal infrastructure counts survive Pod removal.
- [x] 6.2 Package chart and raw manifests from one generated source; verification: install/upgrade/uninstall on supported Kubernetes versions, RBAC audit and suspended examples.
- [ ] 6.3 Run real vendor E2E in a dedicated synthetic environment; verification: registration, two turns, clone/push, concurrent users, interruption and fresh-order recovery with protected test-only capture.
- [ ] 6.4 Perform fault/scale tests and tune the lease from measured p99 startup; verification: scheduling exhaustion, unavailable API, node loss and no repeated submission.

## 7. Acceptance and downstream migration

- [ ] 7.1 Verify every [requirement](specs/runner-lifecycle/spec.md) and record evidence plus limits; sync accepted specs only after approval and implementation verification.
- [ ] 7.2 Prepare a separate downstream migration PR with explicit resource ownership and non-overlapping cutover; verification: suspended install, drain, acceptance and rollback rehearsal.
- [ ] 7.3 Publish reviewed release artifacts and support matrix; verification: signed/pinned artifacts, documented limitations and no unresolved critical security/replay failures.

Verified candidate slices are recorded in [implementation evidence](../../testing/implementation-evidence.md). Remaining task checkboxes deliberately retain their full verification criteria. Do not start live dispatch while intake,
launch fencing and security gates remain incomplete. A working demo is not the production acceptance gate.

Task 3.1 now includes end-to-end native Date translation, immutable receipt offsets and missing-Date partial retries. The 1.33.0, 1.36.0 and 1.37.0 API versions verify positive/negative skew, completion, launch, running preservation, cleanup and replay; pure expiry tests cover the one-hour bounds and classifications. Task 6.2 has current clock-fix build Helm/raw install/update/removal evidence on 1.33.1, 1.36.4 and 1.37.0, including loaded image digest verification, positive/negative RBAC, suspended drain and retained CRDs/namespace. These are tested candidates; release support and vendor acceptance remain separate gates. See the evidence ledger for the exact matrix.

Tasks 3.2/4.1 also pass a 32-order, 128-delivery API batch with competing reconciles, lost create responses
and Pod-loss observation on all three API versions. Completed-receipt recovery fixes the concurrent
credential-admission denial without allowing credential recreation. This partially advances task 6.4;
the 20-order physical synthetic fixture now passes revoked/missing-key rotation, pending scheduling expiry,
control-plane pause, worker stop plus explicit Node decommission/recovery, replay and metadata-only one-POST
checks. It exposed and verified a Recreate/current-generation polling fix. Tasks 2.3/6.4 remain unchecked
for real native revocation/connection and measured registration startup p99; kind default CNI and the
synthetic report do not replace network or vendor acceptance.


Task 7.3 now has private review-bundle build/verification tooling: exact compiler, architecture-specific
source vulnerability gates, compiled-dependency SBOM coverage, signed inventory verification and tamper/trust
rejection. A clean committed candidate and ephemeral-key exercise pass; see the evidence ledger. This is
release preparation, not a production signature or publication. The full native/container, identity, licensing,
support and approval criteria remain pending, so the checkbox stays unchecked.

Tasks 1.2/7.3 also have compiled-module and selected-toolchain notice collection in the candidate producer.
The archive preserves original nested license/patent/notice files, with exact module checksums, binary bindings
and signed inventory hashes. Integrity and provenance boundaries pass; collection does not interpret or approve
licensing. Vendor redistribution, public naming, container notices and release support remain review gates.

Task 5.1 now passes for the npm 11.21.0 image revision: native ARM64 and AMD64 CI verify required
CLI flags, read-only/non-root posture and offline npm use. Local QEMU execution failure is retained as a
compatibility observation, not release acceptance. Task 7.3 retains the observed Critical runtime vulnerability gate. See the [runtime review](../../testing/runtime-vulnerability-review.md).

Task 5.1 also passes for the checksum-pinned npm dependency overrides. Native ARM64 and AMD64 CI
SDK/advisory smoke passes; both local image rescans remove the three fixable bundled-dependency High
findings without suppression. The 27 Critical runtime matches remain in task 7.3. Signing responsibility
is clarified in the [candidate guide](../../workflows/release-candidates.md): KMS belongs to optional
publisher infrastructure and is never an operator installation prerequisite.

Task 5.1 passes for the Debian 13/Python 3.14.8 revision after matching native ARM64 and AMD64 CI
execute the full restricted SDK/CLI smoke, including offline Python venv/pip and the npm advisory probes.
Both final architecture scans retain 25 Critical matches; eight earlier matches disappear and six
newer-version curl matches are added. This advances task 7.3 without clearing its release gate.

Task 5.1 passes for the pinned upstream OpenSSH 10.6p1 revision after matching native ARM64 and
AMD64 CI execute the full restricted SSH/SDK smoke. Automated Syft does not identify the compiled
SSH clients; the pinned signed-source/binary record supplements this inventory gap. The ARM64 scan
retains 24 Critical curl/libcurl matches. Task 7.3 retains runtime vulnerability and release gates.
