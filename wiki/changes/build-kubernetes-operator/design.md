---
type: Technical Design
title: "Operator technical design"
description: "API, process boundaries, crash safety and deployment tradeoffs."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:03:43+07:00"
sources:
  - resource: "https://kubernetes.io/docs/concepts/extend-kubernetes/operator/"
  - resource: "https://book.kubebuilder.io/reference/good-practices"
  - resource: "https://kubernetes.io/docs/concepts/workloads/controllers/job/"
---

# Operator design

All decisions below are proposed. No runtime or CRDs exist yet.

## 1. Supported integration boundary

A Fleet controller reconciles native-orchestrator Deployments, hook configuration, runtime ServiceAccount,
scoped RBAC and security resources. The CLI owns polling/claims; its adapter writes an order receipt
and credential through the Kubernetes API. A separate WorkOrder controller submits and observes the Pod.
Go plus Kubebuilder/controller-runtime is the recommended implementation stack; Python is an alternative
for faster prototyping but gives less alignment with generated Kubernetes API tooling.

[The operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) supports this division.
Use separate controllers for the two Kinds and level-based reconciliation, following
[Kubebuilder guidance](https://book.kubebuilder.io/reference/good-practices).

## 2. Proposed API

Group: `runners.biwsantang.github.io/v1alpha1`. Both Kinds are namespaced.

| Resource | Spec inputs | Controller-owned observations |
| --- | --- | --- |
| `ClaudeRunnerFleet` | External `environmentID`; same-namespace `environmentSecretRef`; digest-pinned `orchestratorImage` and `runnerImage`; orchestrator replicas; scheduling fields; resource requests/memory limit; hook timeout, spawn lease; host config reference; proxy/network configuration; activation and retention policy. | `observedGeneration`; `Configured`, `CredentialsReady`, `NetworkValidated`, `Connected`, `Ready`, `Suspended`, `Degraded`; aggregate infrastructure counts. |
| `ClaudeWorkOrder` | Fleet name/UID; order ID; pool ID; deterministic credential Secret reference; immutable token digest, expiry and operator policy snapshot; receipt state initially incomplete. No JWT or user identity. | `Accepted`, `LaunchAttempted`, `PodObserved`, `Terminal`, `SubmissionUncertain`; Pod name/UID; infrastructure outcome, timestamps, retention deadline and sanitized reason. |

The hook may create receipts and perform the one incomplete-to-complete intake transition only.
Admission validates this exception; fields freeze once accepted. Freeze Fleet UID, pool/order identity,
credential digest, retention inputs and policy snapshot. Only the controller writes status.
Never expose arbitrary PodSpec/command/hostPath or a credential value through either API.
Validate node selectors, tolerations and resource budgets through typed fields. Require requests plus
memory limit, no CPU limit by default. A concrete CR sample follows schema generation, not guessed YAML now.

Proposed defaults: suspended; zero pre-warming; one session per Pod; two native-orchestrator replicas
when enabled, with one common lease configuration. Hook concurrency affects intake, not fleet size.
No global session ceiling. ResourceQuota and node capacity may still delay scheduling.

Use a deterministic per-pool claim resource to reject two managed Fleets for the same environment
within the watched namespace. The claim does not detect pollers in other clusters; cutover is operational.
One namespace per trust boundary is recommended. Cross-namespace references and ownership are rejected.

## 3. Durable hook receipt

1. Validate required inputs and expiry; use supplied server time where available for expiry bookkeeping.
2. Derive resource names from a hash of pool ID plus order ID. Session ID is never the deduplication identity.
3. Create an incomplete WorkOrder CR to obtain its UID. Resolve AlreadyExists only after immutable identity checks.
4. Create an immutable Secret with that CR UID as its owner; compare any collision's owner UID and credential digest.
5. Complete the receipt with a Secret reference and an immutable snapshot of the accepted Fleet policy.
6. For a fresh receipt, return success only after both objects are durably stored and matching;
   retries repair incomplete intake. A matching terminal tombstone returns the prior accepted result
   without recreating a deleted credential Secret or launching again.

The hook does not wait for scheduling. An incomplete receipt cannot launch; abandoned incomplete objects
are reclaimed after expiry. A lost hook response can be redelivered and returns the matching receipt result.
Admission verifies caller role, Fleet state/UID, digest syntax and current policy revision. Redelivery of an
already accepted order must preserve its original snapshot even if Fleet configuration changes.

Read Secret data only through scoped uncached clients; the manager cache excludes Secret bodies.
Do not duplicate JWTs into CR fields, annotations, events or logs. Keep short-lived data in memory only
while checking/creating credentials. The native process alone mounts the environment key; the operator
may need namespace Secret administration, but never mounts that key into runtime sessions.

## 4. One submission, conservative crash recovery

A normal reconcile loop recreates missing desired resources. That behavior is unsafe for consumed credentials.
Use a status compare-and-swap to persist `LaunchAttempted=true` **before** sending exactly one Pod create request.
Only the controller invocation winning that transition can issue the request. Followers observe, never resubmit.
Use deterministic names and check foreign owner UIDs; reject adoption.

| Failure boundary | Recovery |
| --- | --- |
| Before receipt completion | Adapter repairs the Secret/reference; no Pod may exist. |
| After receipt completion, before launch fence | Reconcile normally; one winner consumes the fence. |
| After fence, before POST | Never resubmit. Record uncertainty; vendor recovery requires a fresh order. |
| POST times out or manager dies before recording UID | GET the deterministic Pod name and watch; bind a matching UID if observed. Missing/unknown means no second POST. |
| Pod exits, disappears or is manually deleted | Preserve the record and report its outcome; never recreate for this order. |
| Secret removed after terminal Pod | Tombstone still rejects order recreation until its retention floor. |

Disable automatic retries of the Pod-create POST in the chosen client/transport; test ambiguous outcomes.
A current create invocation may complete after its lease/process is interrupted, so recovery must observe late Pods.
Do not call uncertainty a completed or failed user session. Native control-plane lease recovery provides a new order.
This deliberately trades a potentially missed launch for no repeated operator submission; it is not an exactly-once
user-session execution guarantee. Forced record deletion or administrator tampering is outside the guarantee.

## 5. Direct Pod choice and lifecycle

Use a direct Pod with `restartPolicy: Never`, not a Deployment, StatefulSet or Job for a disposable runner.
The controller must observe disappearance without applying a generic desired-Pod repair loop.
[Kubernetes documents](https://kubernetes.io/docs/concepts/workloads/controllers/job/#handling-pod-and-container-failures)
that Jobs can restart programs despite single-completion configuration. A Pod avoids that additional replacement controller;
platform execution and external control-plane behavior still cannot provide exactly-once session execution.

WorkOrder ownership is Fleet UID. Pod ownership is WorkOrder UID; the credential Secret shares that owner.
All live resources remain in one namespace. A deletion timestamp prevents any new submission or receipt repair.
The Fleet deletion finalizer controls drain/retention before allowing its order ownership to be garbage-collected.

Session Pod: non-root UID/GID, read-only root/hooks/config, all capabilities dropped, no escalation,
`RuntimeDefault` seccomp, no service-account token, no host networking/PID/socket access, fresh bounded emptyDir,
per-session credential mount and approved proxy. No cloud identity or broad Git credential.
Use a pinned native runner with one-session capacity, zero reuse grace, operator-controlled repo-settings confinement
and Anthropic git proxy. Validate architecture and toolchain support during image tests.

Suspend gates hook acceptance and unlaunched receipts; existing Pods continue. Unlaunched receipts expire without submission.
Retention keeps a sanitized tombstone through credential expiry plus clock margin and a post-terminal diagnostics floor.
Set a configured maximum retention too; excessive/malformed expiry is rejected at intake.
Remove bearer Secrets once no Pod can still need them. Delete never-started expired Pods conservatively with UID
preconditions; concurrent startup can still race cleanup, which tests and operations must acknowledge.
Running Pods receive no expiry-based garbage collection. Node loss is reported; it cannot justify reusing an order.
Deletion defaults to drain; explicit abort affects owned Pods only. Finalizers stay while Pods/retention require them.

## 6. Security and readiness

Use namespaced RBAC and distinct manager, hook and runtime identities. The runtime cannot create receipts,
read environment Secrets or call Kubernetes. Restrict hook rights to intake resources; native RBAC cannot enforce
name-prefix/label ownership for dynamic creates, so schema/admission plus dedicated namespaces are required.
Admission must fail closed and be installed before intake or activation.

The default manager watches one configured namespace. Controller cluster privileges are limited to required
bootstrap resources; ordinary reconciliation cannot create cluster roles or modify the CNI/nodepool.
Session users receive no Kubernetes credentials. Treat Fleet writers and image/config publishers as trusted administrators.

Do not infer enforced egress from NetworkPolicy presence. Require a protected, fresh conformance report bound to the
security revision, invalidate it on changes and block new launches when it expires. Report producer rights must be
separate from untrusted sessions. Existing sessions retain the restrictive boundary. An expired report stops new work;
it does not kill active sessions. A report is an operator assertion backed by real tests, not portable auto-detection.

Readiness combines valid configuration, referenced inputs, current network approval and native connected readiness.
Probe/export native state using a bounded local mechanism; report Pod state separately. A Kubernetes health check
cannot assert a runner registered unless documented native evidence supports it.

Session identity verification and private credential exchange are a later integration; do not interpret order expiry
parsing or account routing hints as authentication. No account email goes into default logs or metrics.

## 7. Packaging, migration and rollback

Propose separate manager/hook image and vendor-runtime image, with optional packaging of a restricted proxy.
Pin vendor binary integrity and all release image digests; publish generic AMD64/ARM64 artifacts after compatibility tests.
Do not copy proprietary prototype code. Reimplement the general contract in this repository after license review.

Ship chart and raw/Kustomize installation options generated from one source, plus disabled generic examples.
Downstream GitOps owns Fleet intent and credential/config references; operator ownership controls generated objects.
Cloud ECR/SSM/IAM infrastructure remains outside this operator.

Migration is a separate downstream change: stop the previous orchestrator, allow outstanding hooks and runners to settle,
verify no active provisioning, install a suspended operator Fleet, run synthetic acceptance checks, then activate.
Do not modify existing deployment PRs as part of this planning change.
Rollback suspends new receipt/launch, drains current Pods and removes operator ownership only after cleanup.
Never run old and new provisioners concurrently for the same external environment during rollback.
