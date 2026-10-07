---
type: Repository Documentation
title: "Implementation evidence"
description: "Observed checks and explicit acceptance gaps."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T12:24:00+07:00"
---

# Implementation evidence

This ledger describes the implementation candidate, not production acceptance. Run commands from the feature
worktree. Implementation commit: `a26bed1d33aca0a645a3d8979dc057b316a4aaa0`.
The [GitHub verification run](https://github.com/biwsantang/claude-code-cloud-operator/actions/runs/37575653587)
passes all jobs: unit/race/vet/dependency checks, generated-source parity, Linux AMD64/ARM64 cross-builds and both API-server matrix versions.
The final-source ARM64 manager manifest digest is
`sha256:22e0c09a5bcaa932a06e142cb967c28b8f74d86412b97f1f84b2d6f89ef761b7`.
It installs, upgrades and accepts the suspended example through actual server-side dry-run admission. Final-source Helm uninstall also passes. The isolated cluster was removed after preserving sanitized evidence.

| Check | Observed evidence | Limits |
| --- | --- | --- |
| Unit/race and dependency checks | `go test -race ./...`, `go vet ./...`, `go mod verify` passed. | Pure builders, lifetime bounds, metrics and token transport; not a running cluster. |
| API-server recovery | Fourteen scenarios passed under Kubernetes 1.33.0 and 1.37.0 with the race detector in isolated Linux containers. | Envtest has no scheduler, garbage collector or CNI. |
| Receipt recovery | Concurrent callers, receipt/Secret response loss, mismatched credential and snapshot/status denial pass. | Includes foreign Secret rejection and persisted fence/Pod/terminal status response loss. |
| Launch recovery | Fence crash, late Pod, lost create response and two concurrent reconciles produce no repeated submission. | Real API unavailability, node loss and scheduling exhaustion remain pending. |
| Lifecycle | Suspension gates an unlaunched order; running Pods survive expiry; drain waits and preserves diagnostic retention; terminal replay does not recreate a Secret. | Cluster garbage collection and complete abort/uninstall rehearsal remain pending. |
| Sanitized observations | API tests verify receipts/events do not contain synthetic JWTs and Fleet terminal counts survive Pod removal; metrics unit tests exclude private UID labels. | Infrastructure observations only; vendor outcomes remain unknown. |
| Fleet convergence | Repeated unchanged reconciles preserve Deployment resourceVersion. | Kind 1.37 installed admission is ready; hook/session/manager namespace permission checks deny unauthorized access. |
| Hook runtime | ARM64 image builds and installs the hook under a read-only root with UID 1000. | AMD64 execution smoke pending; both Go binaries cross-build. |
| Vendor runtime | ARM64 private image verifies signing-key fingerprint, manifest signature and binary checksum. Read-only UID 1000 smoke prints Claude 2.1.285, Node 24.21.0, Python 3.11.2 and git 2.39.5; required flags exist. | No real registration, session turn, git push or AMD64 native execution verified yet. |
| Packaging | Helm lint and render pass; raw and Helm resources generated from Kustomize. | Corrected Helm install, upgrade and suspended-Fleet drain/uninstall pass on kind 1.37. CRDs and administrator namespace remain; minimum-version cluster install and active vendor drain remain pending. |
| Enforced network | Isolated kind 1.37.0 with Cilium 1.20.2 and cert-manager 1.21.2 is running. | Two fresh selected Pods deny direct public/private/API/metadata/Pod Identity probes with explicit Cilium policy-denied flows. Approved HTTPS through the proxy passes CA validation, private/IP CONNECT is rejected. This is not downstream approval. |

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

## External gates

Dedicated test-only environment credentials and OAuth registration are not available in this worktree.
Real vendor acceptance, enforced downstream egress, measured downstream startup p99, non-overlapping migration,
reviewed release signatures and vendor redistribution remain incomplete. No live fleet is activated and no
release is published. Session success is never inferred from Pod exit status.
