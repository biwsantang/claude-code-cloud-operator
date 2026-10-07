---
type: Change Proposal
title: "Build a Kubernetes operator"
description: "Plan a reusable lifecycle manager around the documented Claude runner hook."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:02:48+07:00"
---

# Build a Kubernetes operator

## Why

A direct spawn script can provision a runner, but Kubernetes recovery, deletion, configuration ownership
and visibility need a durable lifecycle model. The new operator should make those behaviors declarative
without reimplementing Anthropic's queue or requiring Argo Events/Workflows.

## Baseline decision

[Upstream comparison](../../integrations/existing-projects.md) found a close existing operator.
Evaluate it first against the desired failure and security behavior. Adopt/fork if it fits;
record a justified gap before starting a new controller. The design below is the independent option,
not an already selected fork or a claim that no alternatives exist.

## What changes

Propose a Go/Kubebuilder manager with separate Fleet and WorkOrder controllers, a small spawn-hook adapter,
sanitized durable receipts, credential Secrets and disposable direct Pods. Provide namespaced permissions,
status conditions, lifecycle metrics, safe suspension/deletion, install artifacts and runtime acceptance tests.

## Capabilities added

- `runner-lifecycle`: validated fleet intent, durable order intake, guarded single submission, secret separation,
  non-replacement execution, retention, suspension and observable infrastructure state.

Requirements are in the [delta](specs/runner-lifecycle/spec.md); no current specification exists yet.
This is a behavior proposal, not an implemented capability or an accepted source spec.

## Scope

MVP: one watched namespace, native polling, Anthropic API inference, no pre-warming,
no default fleet concurrency cap and no static inference credential. Placement is configurable.
Prepare portable AMD64/ARM64 artifacts; select exact tested versions during implementation.

Defer other inference providers, environment admin API automation, per-user quotas, warm pools,
private-service token exchange, SCM connectors, persistent workspace reuse and multi-cluster coordination.
The name does not promise a self-hosted Anthropic control plane or Claude UI.

## Impact

- APIs: two proposed `v1alpha1` CRDs with status subresources; alpha compatibility may change before release.
- Code: new Go controller project and hook adapter; no application implementation in this change.
- Dependencies: controller-runtime/Kubebuilder and the vendor CLI; exact supported versions remain to be pinned.
- Risks: lost launches after uncertain writes, incomplete network validation, credential visibility to administrators,
  beta protocol changes, drain delays and unpushed work loss on infrastructure failure.

See [research](../../integrations/anthropic-contract.md), [design](design.md), [tasks](tasks.md)
and the [test strategy](../../testing/index.md). Next action: **apply** after design review.
