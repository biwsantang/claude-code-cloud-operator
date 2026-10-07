---
type: Implementation Plan
title: "Operator implementation plan"
description: "Ordered tasks with observable completion and release gates."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:02:48+07:00"
---

# Implementation tasks

All tasks are pending. Documentation research does not complete runtime implementation.

## 1. Contract and project foundation

- [ ] 1.1 Evaluate the existing operator against crash/replay/security requirements; verification: record upstream gaps and select adoption, licensed fork or independent implementation before scaffolding.

- [ ] 1.2 Choose license and supported Go/Kubebuilder/Kubernetes/Claude versions; verify compatibility, binary integrity and redistribution policy before selecting release pins.
- [ ] 1.3 Scaffold Go manager and hook commands; verification: build and unit test on AMD64/ARM64, dependency lock and credential-free CI.
- [ ] 1.4 Add CRD types, defaults, status subresources and admission; verification: generated schema, rejected unsafe/cross-namespace/mutable-order inputs and gated intake until admission is ready.

## 2. Fleet reconciliation

- [ ] 2.1 Reconcile namespace-scoped Deployment/config/RBAC/service accounts and pool claim; verification: repeat reconciles converge without touching unrelated resources.
- [ ] 2.2 Reconcile suspended defaults and protected network report validation; verification: missing/stale/mismatched reports block activation and new launches.
- [ ] 2.3 Implement native connection status and credential-revision rollout; verification: revoked/missing credentials degrade status, rotation restarts polling without altering accepted runners.

## 3. Durable intake

- [ ] 3.1 Implement adapter input checks and sanitized receipt naming; verification: missing/expired/malformed inputs and permanent/transient error classifications.
- [ ] 3.2 Persist incomplete CR, immutable owned Secret and accepted snapshot; verification: concurrent redelivery, response loss, ownership collisions, partial-write repair and no JWT leakage.
- [ ] 3.3 Restrict actor roles and one-time receipt completion through admission; verification: untrusted updates, Fleet UID mismatch and snapshot changes denied.

## 4. Work-order lifecycle

- [ ] 4.1 Implement CAS launch fence and single Pod-create path with transport retries disabled; verification: two controllers and crashes at every write boundary never resubmit an order.
- [ ] 4.2 Observe late/missing/terminal Pods and immutable UIDs; verification: ambiguous API outcomes and forced Pod deletion produce no replacement Pod.
- [ ] 4.3 Implement credential removal, tombstone floors, capped retention and conservative pending cleanup; verification: replay after cleanup, expired orders, running/startup races and projected token rotation.
- [ ] 4.4 Implement suspend, drain/abort deletion and finalizers; verification: scoped cleanup, waiting reasons, controller restart and uninstall instructions.

## 5. Runtime and security

- [ ] 5.1 Build immutable non-root runtime and hook images with pinned native CLI; verification: verified downloads, toolchain/version tests, read-only smoke and updater disabled.
- [ ] 5.2 Generate Pod security/storage/resources and configurable placement; verification: no API/environment key/cloud identity access from sessions and no fleet concurrency cap introduced.
- [ ] 5.3 Establish proxy/egress integration and network acceptance report format; verification: direct/proxied denial and fresh-Pod startup tests on enforced-network clusters, not envtest alone.

## 6. Operations and delivery

- [ ] 6.1 Add status/events/metrics without sensitive values or high-cardinality user labels; verification: redaction tests and terminal infrastructure counts survive Pod removal.
- [ ] 6.2 Package chart and raw manifests from one generated source; verification: install/upgrade/uninstall on supported Kubernetes versions, RBAC audit and suspended examples.
- [ ] 6.3 Run real vendor E2E in a dedicated synthetic environment; verification: registration, two turns, clone/push, concurrent users, interruption and fresh-order recovery with protected test-only capture.
- [ ] 6.4 Perform fault/scale tests and tune the lease from measured p99 startup; verification: scheduling exhaustion, unavailable API, node loss and no repeated submission.

## 7. Acceptance and downstream migration

- [ ] 7.1 Verify every [requirement](specs/runner-lifecycle/spec.md) and record evidence plus limits; sync accepted specs only after approval and implementation verification.
- [ ] 7.2 Prepare a separate downstream migration PR with explicit resource ownership and non-overlapping cutover; verification: suspended install, drain, acceptance and rollback rehearsal.
- [ ] 7.3 Publish reviewed release artifacts and support matrix; verification: signed/pinned artifacts, documented limitations and no unresolved critical security/replay failures.

First implementation slice: baseline evaluation, then tasks 1.2–1.4, then a suspended Fleet. Do not start live dispatch while intake,
launch fencing and security gates remain incomplete. A working demo is not the production acceptance gate.
