---
type: Repository Documentation
title: "Proposed architecture"
description: "Supported native polling plus Kubernetes fleet and order reconciliation."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:00:24+07:00"
---

# Proposed architecture

There is no implemented architecture yet. The [design](../changes/build-kubernetes-operator/design.md)
proposes two namespaced APIs: `ClaudeRunnerFleet` for installation configuration and `ClaudeWorkOrder`
for durable receipt and lifecycle of one spawn request.

```mermaid
flowchart LR
  A[Anthropic control plane] <-->|outbound HTTPS| O[Native orchestrator]
  O --> H[Spawn hook adapter]
  H -->|CR plus credential Secret| K[Kubernetes API]
  K --> C[Operator controllers]
  C -->|one create attempt| P[Disposable runner Pod]
  P -->|outbound HTTPS| A
  G[GitOps fleet configuration] --> K
```

The hook returns after durable receipt, not after Pod startup. The operator reconciles receipt,
submission, observed Pod state and cleanup. Kubernetes running state does not prove Anthropic registration.
Generic installation tooling supplies CRDs and the manager; downstream GitOps owns only fleet intent
and referenced inputs. It must not manage generated runtime resources independently.
