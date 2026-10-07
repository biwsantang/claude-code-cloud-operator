---
type: Repository Documentation
title: "Documentation map"
description: "Navigate research, operator design and pending delivery work."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:59:38+07:00"
---

# Documentation map

The implementation candidate is under active verification. Read in this order:

1. [Research and source ledger](integrations/anthropic-contract.md): verified external contracts and unknowns.
2. [Existing projects](integrations/existing-projects.md): choose adoption, fork or independent implementation.
3. [Proposal](changes/build-kubernetes-operator/proposal.md): goals and scope.
4. [Design](changes/build-kubernetes-operator/design.md): APIs, controllers and recovery.
5. [Requirements](changes/build-kubernetes-operator/specs/runner-lifecycle/spec.md): observable acceptance behavior.
6. [Tasks](changes/build-kubernetes-operator/tasks.md): ordered implementation gates.

Supporting pages: [architecture](architecture/index.md), [terminology](concepts/index.md),
[operations](operations/index.md), [test strategy](testing/index.md),
and [contributor workflow](workflows/index.md).

The active change is **apply `build-kubernetes-operator`**.
Review [implementation evidence](testing/implementation-evidence.md) and the
[installation/drain guide](operations/install-and-drain.md). Production rollout remains gated.

- [Operator practices and community lessons](integrations/operator-practices.md)
