---
type: Repository Documentation
title: Existing operator alternatives
description: Compare primary upstream documentation before choosing a fresh implementation.
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:59:38+07:00"
sources:
  - resource: https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/README.md
  - resource: https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/docs/on-demand.md
  - resource: https://github.com/yuriyostapenko/shock/blob/a237cf3db5a0035555533ba447519d41ef64b023/README.md
---

# Existing projects and adoption decision

Inspected public README documentation and the first project's on-demand design on 2026-10-07.
These are upstream author descriptions, not independently reproduced runtime verification.

| Option | Observed fit | Decision consequence |
| --- | --- | --- |
| [Claude self-hosted environment operator](https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/README.md) | Pre-release Go operator with `ClaudeEnvironment`/`ClaudeRunner`, fixed and on-demand modes, hardened direct Pods, Helm/Kustomize and native hook integration. Its [on-demand documentation](https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/docs/on-demand.md) describes non-replacement after Pod loss and optional soft concurrency limits. | Closest baseline. Test receipt partial writes, uncertain Pod-create outcomes, replay retention and activation policy before deciding whether to adopt or fork. |
| [SHOCK](https://github.com/yuriyostapenko/shock/blob/a237cf3db5a0035555533ba447519d41ef64b023/README.md) | Helm plus Go controller around agent-sandbox, persistent session PVCs, sleep/wake lifecycle, Kubernetes 1.35+ and a documented default active-session cap of two. | Useful reference for conditional writes and lifecycle tests. Its default persistence and capacity model differ from this proposal's disposable execution and uncapped fleet. |
| Implementation candidate in this repo | WorkOrder receipt, launch fence, tombstone retention and required network approval; narrow initial scope. Implemented and tested in the private candidate; production acceptance remains pending. | More development and maintenance work. Proceed only if upstream evaluation identifies a justified gap or a deliberate independent-project goal. |

GitHub reports both projects as Apache-2.0. Review the actual license, notices and contribution policy
before importing code. No upstream application code is copied. The selected baseline is an independent Apache-2.0 implementation.

## Recommended gate before implementation

Evaluate the first operator against this plan's failure-boundary and network tests.
If it meets the requirements, adopt it downstream or maintain a small licensed fork here.
If changes are needed, compare an upstream contribution/fork against a new implementation.
Record concrete gaps and choose the baseline before scaffolding.

The API names and launch-fence design are the independent implementation candidate; adopting an upstream API
requires updating the proposal and delta before implementation. Creating this repository does not justify
maintaining another controller by itself. SHOCK is an alternative if persistent workspaces later become a goal.

## Implementation baseline decision (2026-10-07)

Selected independent implementation after inspecting the pinned hook, runner controller and license.
The upstream project provides useful patterns, but the changes below affect its receipt API and core
lifecycle rather than a small deployment overlay. A fork would carry fixed-runner modes outside our scope.
This is a source review; no upstream runtime test has been reproduced.

- Its [hook](https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/internal/hook/hook.go) creates the bearer Secret before the CR, then changes ownership best effort. An existing receipt returns success before comparing credential identity. Our receipt-first transaction requires a different accepted-state contract.
- Its [runner controller](https://github.com/AhmadMasry/claude-self-hosted-environment-operator/blob/a4268377b178d9f92c2e8074e1fcf71833544337/internal/controller/clauderunner_controller.go) calls Pod Create before persisting PodName. A crash or lost status write leaves a retry window if that Pod disappears. Our durable pre-POST fence intentionally sacrifices retry availability.
- The same controller expires terminal records from finished time (default 300 seconds). Our tombstone floor also includes bearer expiry and skew. Its good non-replacement behavior after persisted PodName remains a useful comparison.

Regression evidence required here: simultaneous intake, lost Secret/Create responses, launch-fence crash,
late Pod observation, forced deletion, replay after Secret cleanup, and network report invalidation.
Apache-2.0 covers new operator code and generated Kubebuilder scaffolding. Vendor software keeps its own
terms; initial releases will not redistribute vendor binaries until that separate gate is cleared.
