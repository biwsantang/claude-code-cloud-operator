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

Helm `fleet.session` maps to typed `spec.execution.lifecycle`, then to the accepted WorkOrder.
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
named native environment variables use milliseconds. The chart exposes the typed units only.
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

Two orchestrator replicas improve polling availability, not total concurrent session capacity. Use
Anthropic organization capacity/budget, namespace ResourceQuotas, node autoscaling and suitable CPU/
memory requests to bound aggregate usage; there is no custom quota controller. Defaults are 8Gi
workspace, 1 CPU and 2Gi memory requested, 4Gi memory limit, with no CPU limit.

Suspend stops new intake and leaves active sessions running. Drain stops intake and waits for native
finish/release before deleting resources. Abort requests Pod deletion and can interrupt work after its
termination budget. Credential expiry bounds launch validity and retention of replay evidence;
diagnostic retention controls receipts, neither is a session inactivity clock.

## Helm Fleet setup

`fleet.enabled: false` preserves operator-only installation and external Fleets. To opt in, supply
existing environment Secret and network report references, environment ID, proxy selectors/URL,
security revision, and tested digest-pinned runtime/hook images. The release packages its own compatible
hook digest; the source chart requires a tested hook. No organization runtime or environment key is
invented. Optional `hostConfigRef` points to an existing immutable namespace ConfigMap; version its
name when changing administrator settings. Existing image configuration and administrator files are
materialized as regular files, with executable hook modes retained, before native startup snapshots them.

With direct Helm, install the operator with Fleet disabled and `--wait`, verify certificate and actual
admission dry-run, then `helm upgrade ... -f fleet-values.yaml` with Fleet enabled and suspended.
For that Fleet-enabled step, use `--wait=legacy` on Helm 4 (Helm 3 uses `--wait`), or explicitly wait
only for the manager Deployment/certificate. Helm 4's default watcher can wait forever for a deliberately
suspended Fleet's `Ready=False`; installation readiness and active intake readiness are different.
An initial Fleet-enabled direct Helm install can race the fail-closed webhook; use the two-step path.
For Argo CD, the generated Fleet uses sync wave 20 and skips pre-CRD dry-run, after lower-wave operator
resources are healthy. Keep the initial `fleet.suspended: true`. Reproduce network conformance, bind
the report to the actual Fleet UID/declared policy digest, then explicitly set suspended false.

Fleet prune/delete and Helm retention protections remain enabled. Retention does not mean draining:
explicitly suspend/drain Fleets before removing the operator. If disabling chart Fleet management,
its retained object remains administrator-owned until explicitly removed. On upgrades, suspend intake,
apply compatible CRDs and update the Fleet hook to this operator version before resuming new intake;
older hooks lack the new configuration helper. Keep pinned runtime qualification separate from chart
publication. See [public installation](public-installation.md) and [drain instructions](install-and-drain.md).

The normal chart does not expose arbitrary native args/env, per-Pod capacity, internal timeouts or
owner-lock overrides. Advanced compatible execution placement remains in the typed Fleet API.

See the [dedicated native lifecycle trial](../testing/session-lifecycle-trial.md) for observed behavior and remaining gates.
