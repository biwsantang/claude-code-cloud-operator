---
type: Repository Documentation
title: "Core concepts"
description: "Distinguish fleets, orders, sessions and credentials."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:59:38+07:00"
---

# Core concepts

| Term | Meaning in the proposal |
| --- | --- |
| Fleet | A configured Kubernetes installation linked to one externally created environment. |
| Environment | Anthropic-managed pool; not a Kubernetes namespace or an access-control list. |
| Work order | One spawn request with an opaque idempotency identity and short-lived credential. |
| Session | User conversation that can generate multiple work orders over its lifetime. |
| Receipt | A durable Kubernetes record that the hook can safely acknowledge. |
| Launch fence | Persisted permission consumed before a single Pod create attempt; recovery cannot reuse it. |
| Tombstone | A retained order record preventing recreation after its Pod or credential is removed. |

Do not equate Pod Running, runner registration and a completed user session.
A Pod's terminal outcome is an infrastructure observation; session outcome needs separate native evidence.
One-session capacity is an isolation choice and does not impose a fleet-wide concurrency ceiling.
