---
type: Repository Documentation
title: "Shared environment session lifecycle"
description: "Curated lifecycle controls for ephemeral native sessions, with frozen receipt compatibility."
tags: [claude-code, lifecycle, helm, operations]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-09T12:30:00+07:00"
sources:
  - resource: https://code.claude.com/docs/en/self-hosted-environments-reference
  - resource: https://code.claude.com/docs/en/self-hosted-environments-configuration
  - resource: repo://internal/contract/lifecycle.go
  - resource: repo://internal/builders/pod.go
---

# Shared environment session lifecycle

This source follow-up is not included in the published `v0.1.0-rc.2` artifacts.

One Fleet represents a shared Anthropic environment for authorized organization members. Native
Anthropic polling, assignments and session state remain authoritative. A completed response does
not end the session: subsequent turns can use the same Pod and workspace until native release.
Every disposable session Pod has native capacity 1, drain grace 0, `restartPolicy: Never`, bounded
`emptyDir` storage, non-root/read-only execution, network isolation and no Kubernetes SA token.
The durable WorkOrder's single-launch fence prevents relaunching its old assignment.

## Small set of controls

The administrator-owned Fleet declares typed `spec.execution.lifecycle`, which is frozen into each
accepted WorkOrder.
These are session settings, independent of manager configuration and credential expiry.

| Setting | New-order default | Meaning |
| --- | --- | --- |
| `idleMinutes` | 30 | Native idle release after the turn ends or the user is idle at a permission prompt; unfinished work can delay release. 0 disables. |
| `maxSessionMinutes` | 480 | Native release threshold; current tested native 2.1.285 adds a 15-minute grace before its hard cap. This is not an exact eight-hour kill. 0 disables. |
| `shutdownWaitSeconds` | 300 | On first SIGTERM, stop new work and allow in-flight work to finish for this budget before stopping the child. 0 skips the wait. |
| `promptToSave` | true | A once-per-turn Stop reminder for intended uncommitted or unpushed work; observes local Git only. |
| `pushOutcomeOnRelease` | false | Native best-effort push of tracked committed outcome branches on supported runner-initiated release paths. |

Minutes are integers 0..10080 and shutdown seconds 0..86400. Positive idle time cannot exceed a
positive maximum age. An economical example is 15/240 minutes; a longer workday can use 60/720.
Neither is a named profile or a durability promise. Native CLI flags use minutes/seconds; similarly
named native environment variables use milliseconds. The Fleet API exposes these typed units.
[Native flag and shutdown reference](https://code.claude.com/docs/en/self-hosted-environments-reference)

The Pod's termination grace is `shutdownWaitSeconds + 120`: 420 seconds by default. The fixed native
stop/post-session budgets are pinned at 5/60 seconds. Native overhead is 80 seconds; its startup message
also allows up to 20 seconds for a release already in progress. When release pushing is enabled its
additional 30 seconds require `shutdownWaitSeconds + 140` (440 by default), retaining 10 seconds of
headroom even during that release race. Confirm the actual startup budget whenever
qualifying another native version; rendered arguments alone do not prove native behavior. A second
signal, forced Pod deletion or node loss can interrupt cleanup and cannot guarantee saving.

## Saving and resume

The Stop reminder asks Claude to follow user instructions, select only intended files, exclude secrets
and scratch files, and obtain any required approval. User refusal is respected. It never performs
`git add`, commits, pushes, force-pushes or merges itself. The native `stop_hook_active` guard suppresses
reentry in that turn. It is quiet for a clean repository, no Git/origin, malformed input or Git failure.
Image hooks and administrator Stop hooks remain installed; `disableAllHooks: true` is respected.

Before enabling release pushing, an administrator must restrict who can update the repository's
`claude/*` outcome refs. Native resume can fetch those refs, so allowing unrelated collaborators to
write them expands the trusted code surface. Release pushing is best effort and saves committed
tracked outcome work only. It does not back up uncommitted files or checkout-hook repositories.
[Native configuration and outcome security](https://code.claude.com/docs/en/self-hosted-environments-configuration)

After idle release, the next user message can resume the transcript on a **fresh native assignment,
WorkOrder and Pod**. Its filesystem is fresh. Transcript resume does not recover an `emptyDir`; no PVC
or session-to-volume routing is introduced. Commit and approved push intended work before release.
[Kubernetes emptyDir lifecycle](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir)

New orders freeze all resolved settings and the session configuration helper image. Old stored orders
without lifecycle fields keep their old digest, native arguments and 120-second Pod termination grace;
schema/defaulting does not rewrite them. Lifecycle/helper additions do not change network selectors.
Existing receipt validation, current proxy/report checks and the launch fence still apply.

## Organization controls and operations

Anthropic organization/environment membership controls who can dispatch. Repository integration
permissions control repository access. An account email or Kubernetes label is not authentication.
Environment Secrets are administrator-managed and never exposed in Helm values or to session Pods;
the disposable runner receives only its immutable assignment credential.

Each Fleet represents one Claude environment and references its own environment Secret. One shared
operator can manage multiple distinct environments; see [multiple environments and credentials](public-installation.md#multiple-environments-and-credentials)
and the [two-Fleet example](../../examples/multiple-environments/README.md). Session runners receive
assignment credentials rather than the shared environment key.

Two orchestrator replicas improve polling availability, not total concurrent session capacity. Use
Anthropic organization capacity/budget, namespace ResourceQuotas, node autoscaling and suitable CPU/
memory requests to bound aggregate usage; there is no custom quota controller. Defaults are 8Gi
workspace, 2 CPUs and 4Gi memory requested, 4Gi memory limit, with no CPU limit.
These resource defaults are resolved only for Fleets; complete legacy budgets remain unchanged.

Suspend stops new intake and leaves active sessions running. Drain stops intake and waits for native
finish/release before deleting resources. Abort requests Pod deletion and can interrupt work after its
termination budget. Credential expiry bounds launch validity and retention of replay evidence;
diagnostic retention controls receipts, neither is a session inactivity clock.

## Operator installation and user Fleet configuration

The Helm chart installs only the operator, RBAC, admission/certificate resources and CRD definitions.
It does not create a Fleet or accept environment credentials/session policy in its values. Source installations
supply manager/hook images; follow-up release packaging supplies both digests. Installation values provide
shared runtime and network defaults while the Fleet remains a user-owned manifest.

The [minimal Fleet](../../config/samples/runners_v1alpha1_clauderunnerfleet.yaml) needs its environment ID
and existing Secret reference. Optional lifecycle, resources and custom runner images stay on the Fleet.
See [default resolution and network acceptance](fleet-defaults.md). Optional `execution.hostConfigRef`
points to an existing immutable namespace ConfigMap; version its name when changing settings.

With direct Helm, install the operator with `--wait`, verify the certificate and actual admission dry-run,
then apply the separate Fleet manifest with `kubectl`. Keep `spec.suspended: true` initially. Reproduce
network conformance, bind the report to the actual Fleet UID/declared policy digest, then explicitly set
suspended false. The Fleet is outside the Helm release, so operator installation does not wait for a
suspended Fleet to report ready.

For Argo CD, keep the operator Helm Application and administrator-owned Fleet/input manifests separate.
Sync the operator and verify admission first; then sync the suspended Fleet and complete the same
network acceptance procedure. The example's prune/delete annotations protect its lifecycle; they do not
drain it. Explicitly suspend/drain Fleets before removing the operator. Helm uninstall does not own or
delete user Fleets.

On upgrades, suspend intake, apply compatible CRDs and update the installation hook to this operator version (or any explicit legacy Fleet override)
before resuming new intake; older hooks lack the new configuration helper. Keep pinned runtime
qualification separate from chart publication. See [public installation](public-installation.md) and
[drain instructions](install-and-drain.md).

The operator chart has no `fleet.enabled` switch. Per-session lifecycle/resources and compatible
execution placement belong in the typed Fleet API. There is no implemented Fleet concurrency cap or
queue-expiration setting; orchestrator replicas and startup leases are not substitutes for those controls.

See the [dedicated native lifecycle trial](../testing/session-lifecycle-trial.md) for observed behavior and remaining gates.
