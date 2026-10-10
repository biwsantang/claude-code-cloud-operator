---
type: Repository Documentation
title: "Verification strategy"
description: "Layered acceptance gates for future operator implementation."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T18:44:19+07:00"
sources:
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-testing"
  - resource: "https://kind.sigs.k8s.io/docs/user/auditing/"
  - resource: "https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/"
---

# Verification strategy

The implementation and its verification are in progress. Read [observed evidence and limits](implementation-evidence.md); the table below defines acceptance layers rather than claiming they are all complete.

| Layer | Planned evidence |
| --- | --- |
| Unit | Field/default validation, receipt collisions, error classification, token redaction, retention calculations and launch-fence transitions. |
| API server / envtest | Concurrent reconciliation, status CAS conflicts, immutable fields, owner UIDs, Secret partial writes, finalizers and API failure injection. |
| Kind / cluster | RBAC denials, operator restart at every write boundary, forced Pod deletion without recreation, Secret rotation, API TLS verification and namespace isolation. |
| Enforced-network cluster | Direct/proxied egress deny cases including metadata, internal services, API access and fresh-Pod startup; invalidate activation on policy changes. |
| Vendor E2E | Dedicated environment, registration, dispatch, follow-up, Git clone/push, concurrent users, lost runners and fresh-order recovery. |
| Release | AMD64/ARM64 build matrix, binary integrity, no runtime updater, digest references, chart/manifests and dependency scanning. |

[Vendor E2E guidance](https://code.claude.com/docs/en/self-hosted-environments-testing) requires cloud-session
OAuth and a test-only capture path. Protect its transcripts and use synthetic prompts/repositories.
Do not add a capture hook or a transcript collector to production images.

Kill the manager before and after each status/Secret/Pod write. Pass only if a repeated order cannot
produce a second Pod creation attempt after an ambiguous outcome; accepting a missed launch is deliberate.
Also exercise Secret-owner mismatch, cancelled orders, denied API calls and expired orders after process restart.
Tests of a fake API alone cannot certify scheduling, network enforcement or registration.

## Reproduce physical synthetic faults

The [`fault-smoke` harness](../../hack/fault-smoke.py) creates and removes its own two-node kind cluster.
Use the Linux architecture matching the kind host; this example is ARM64. Run from the repository worktree:

```sh
docker buildx build --platform linux/arm64 --target manager --load -t claude-cloud-manager:fault .
docker buildx build --platform linux/arm64 --target hook --load -t claude-cloud-hook:fault .
docker buildx build --platform linux/arm64 -f hack/fault-fixture/Dockerfile --load -t claude-cloud-fault-fixture:dev .
python3 hack/fault-smoke.py --kind "$(command -v kind)" \
  --manager-tag claude-cloud-manager:fault --hook-tag claude-cloud-hook:fault \
  --fixture-tag claude-cloud-fault-fixture:dev --evidence /tmp/claude-fault-smoke.json
```

The harness refuses an existing cluster and requires a dedicated cluster-name prefix and a labelled
synthetic runtime image. It verifies loaded OCI targets before installation. Its fixed synthetic
environment key and fake work orders do not authorize vendor dispatch. The runtime never calls Anthropic.
Its bound report is explicitly a synthetic prerequisite assertion, not network conformance or downstream approval.

Scenarios cover multiple distinct running orders/redelivery, simulated revoked/missing-key readiness and
credential rollout, exhausted scheduling through worker cordon, conservative pending expiry, control-plane
pause with hook retry, physical worker stop plus explicit Node decommission/orphan cleanup, fresh Node/order
recovery and Abort with retained tombstones. Default kind CNI does not certify NetworkPolicy enforcement.
Real vendor revocation, registration and production startup p99 remain separate gates.

[Kind auditing](https://kind.sigs.k8s.io/docs/user/auditing/) supplies the setup pattern. This fixture uses
metadata-only audit rules scoped to the manager's Pod-create requests, with no request or response bodies.
The [v1beta4 kubeadm API](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/) uses named
extra-argument entries rather than the older guide's map. Shared-home scratch files are checked inside a
container before boot, avoiding Colima's unshared macOS temporary path; Helm configuration/cache are isolated.
The final evidence must show one successful audited Pod-create request per order before the run can pass.

Review [container vulnerability findings and execution limits](runtime-vulnerability-review.md) before accepting the optional runtime. Operator release checks are described in the [release guide](../workflows/release-candidates.md).

Review the [curl advisory assessment](curl-advisory-assessment.md) for the eight retained advisory IDs and missing regressions.

The [dedicated native trial](implementation-evidence.md#dedicated-native-vendor-trial-and-intake-fixes) records actual registration, OAuth/session turns, tools and recovery with explicit remaining gates.

The [native lifecycle follow-up trial](session-lifecycle-trial.md) records accelerated idle release,
workspace continuity, reminder behavior and graceful shutdown, with publication/runtime limits.
