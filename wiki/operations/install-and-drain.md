---
type: Repository Documentation
title: "Install and drain"
description: "Namespace trust, bootstrap and safe removal."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T13:26:03+07:00"
---

# Installation and lifecycle operations

Use one dedicated namespace and one installation per cluster. The manager watches that namespace only.
Ordinary permissions are namespaced; its one cluster read checks the named admission configuration.
Bootstrap CRDs, admission registration and cert-manager resources require administrator installation rights.
The hook can get its Fleet, create/get/update receipts and create credential Secrets; it cannot read Secrets,
write status, create Pods or edit protected network reports. Runtime service accounts have no bindings or token.
Do not associate session identities with cloud roles. Keep image/config/Fleet/report writers trusted.

## Before activation

1. Build privately and verify manager, hook and vendor runtime candidates; pin image digests.
2. Install cert-manager, create the trust namespace and apply Restricted Pod Security labels.
3. Install the chart with a tested manager digest, or render the raw source after changing its placeholder image.
4. Wait for manager readiness and test installed admission. Credential webhook request filtering must remain intact.
5. Create a suspended Fleet with same-namespace references. Create the environment key externally under Secret
   key `environment-secret`; never place it in a Fleet, chart value, example or repository.
6. Provide an immutable host ConfigMap if required, and a controlled proxy with an explicit public destination
   allowlist that rejects Kubernetes, private services, metadata/cloud identity and redirect bypass.
7. Reproduce all required network tests from fresh Pods with the Fleet's policy labels. A trusted producer writes
   a ConfigMap with `report.json` matching `internal/contract.Report`. Bind Fleet UID, policy digest, evidence,
   test time and expiry (at most 24 hours). Untrusted sessions and hooks must have no report-write permission.
8. Complete dedicated vendor acceptance. Activate only after these gates pass. Missing/stale reports or changed
   policy stop polling/new launches; active execution retains its existing policy.

Synchronize the hook and manager nodes' clocks and monitor drift. The adapter translates the native poll's
HTTP Date into this cluster clock domain and freezes that offset per receipt. Offsets beyond one hour cause
a retryable `ClockSkewOutsideBudget` failure; repair the clock before retrying. A missing Date on fresh intake
uses local time; a partial retry keeps the original offset. Signed expiry is retained unchanged and cleanup
never shortens its floor because of translation. This does not synchronize clocks across nodes.

Avoid mutable configuration names. Publish immutable ConfigMaps under new names; retain referenced versions
until their orders finish. Old policy revisions remain while the Fleet exists to preserve active boundaries.
Source TLS certificate handling is generic and separate from inference credentials.

After `helm --wait`, verify the Certificate is Ready and the webhook Service has ready EndpointSlices, then
run `kubectl apply --dry-run=server -k config/samples` using the intended kubeconfig/context. A newly created
Service can briefly reject connections while routing converges, even after manager readiness. Retry this
side-effect-free check after the endpoint settles; do not disable admission to bypass a startup error. The
final-image kind test reproduced this transient and then passed the dry run. Dry-run success proves the
installation path, not vendor or network acceptance.

## Suspend, delete and recover

Set `spec.suspended: true` to stop intake and new launches. Running session Pods continue. An already fenced
in-flight create can finish; suspension is not a transaction across Fleet and WorkOrder writes.
Default deletion policy is Drain: stop polling, wait for owned active execution, remove credentials safely and
retain receipts through bearer expiry plus clock margin and the diagnostic floor. This may take hours.
Trusted administrators can explicitly choose Abort before deleting to terminate owned execution. Neither
mode deletes unrelated resources. Inspect sanitized conditions and retained order reasons when cleanup waits.

Do not delete CRDs, force-remove finalizers, delete receipt tombstones or uninstall the manager before drain
finishes. Those administrator actions are outside the one-submission guarantee. If the manager was removed,
reinstall the compatible candidate, restore admission and finish cleanup before uninstalling again. Helm retains
CRDs and leaves the namespace to its administrator; never make namespace deletion the ordinary rollback path.

Rollback: suspend, stop old/new polling overlap, drain existing orders, then revert the implementation under
review. A lost or uncertain order requires a fresh order from the external control plane, not a replacement Pod.
The operator reports infrastructure state; it cannot report a user's session outcome from Kubernetes status.

Metrics are disabled by default. If `--metrics-bind-address` is enabled, isolate the endpoint to trusted
monitoring clients. Its fixed phase labels expose retained infrastructure counts, including terminal observations
through Pod cleanup. Native/vendor logs and metrics require their own access and cardinality review.

## Reproduce packaging checks

In a fresh disposable `kind-claude-operator-...` cluster, install CNI/cert-manager, load the verified manager
image and pin its actual loaded target digest. Run:

```sh
python3 hack/install-smoke.py \
  --kubeconfig PATH --context kind-claude-operator-NAME \
  --manager-image REPOSITORY@sha256:DIGEST --evidence RESULT.json
```

The fixture
verifies the digest, both installation paths, RBAC denials/positive grants, suspended convergence and drain,
and removal while retaining CRDs/the namespace. It creates no environment key or activation report. The
caller removes the disposable cluster after preserving evidence. Never run this fixture on a live installation.
