---
type: Repository Documentation
title: "Deployment and operations plan"
description: "Activation, ownership, rotation and rollback prerequisites."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T14:05:45+07:00"
---

# Operations plan

The candidate provides generated CRDs, namespaced RBAC, manager manifests, suspended examples and a chart.
See the [installation/drain guide](install-and-drain.md) and [exact tested matrix](../testing/implementation-evidence.md).
Release and vendor acceptance remain pending; cluster-scoped CRD installation requires a cluster administrator.
Start with one manager installation watching one explicit namespace. Multiple namespaces need separate,
reviewed RBAC and independent fleet environments; multi-cluster coordination is outside the MVP.

A trusted operator supplies the external environment ID, credential Secret, digest-pinned images,
placement, resource budgets and network validation. The fleet starts suspended. Kubernetes CR writers
are infrastructure administrators, not arbitrary session users; control of image/config references permits code execution.
No cloud identities, account IDs or organization labels are built into defaults.

Before enabling, verify Secret sync, image integrity, restrictive pod security and enforced egress including
proxy bypass, DNS, private destinations and metadata. A NetworkPolicy object's existence cannot prove enforcement.
Use a protected, current network-test report bound to the fleet's policy/config revision; changes invalidate approval.
The manager refuses activation without that report. A downstream controller may produce it, or an administrator
may record independently verified results. This is a deployment trust input, not proof discovered from CNI manifests.

Rotate the environment credential by restarting native orchestrators on Secret revision changes;
existing sessions keep their work-order lifecycle. Never rotate a consumed work-order Secret in place.
Measure queue age, receipt/submission failures, pending startup age, connection state and cleanup backlog.
Do not use session IDs or emails as metric labels. Aggregate observed infrastructure terminal outcomes separately
from user-session results. Authenticated exporter access and external metrics collectors remain install-time configuration.

Suspend stops receipt admission and new launches while preserving existing Pods. Fleet deletion drains and retains
records until their retention floor; a finalizer reports why deletion is waiting. An explicit abort policy terminates
owned Pods only after a trusted administrator chooses it. Operator uninstall with active finalizers can strand resources;
drain and remove fleets before uninstalling controllers or CRDs. Forced finalizer removal bypasses guarantees.

Roll back by suspending, keeping the network boundary in place, reverting future image/configuration,
and allowing current runners to finish. Never bring old and replacement provisioning systems online for the same
pool during cutover. These procedures require runtime verification before release.
