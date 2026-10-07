---
type: Repository Documentation
title: "Verification strategy"
description: "Layered acceptance gates for future operator implementation."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:59:38+07:00"
sources:
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-testing"
---

# Verification strategy

The repository currently has documentation only. No runtime tests have run.

| Layer | Planned evidence |
| --- | --- |
| Unit | Field/default validation, receipt collisions, error classification, token redaction, retention calculations and launch-fence transitions. |
| API server / envtest | Concurrent reconciliation, status CAS conflicts, immutable fields, owner UIDs, Secret partial writes, finalizers and API failure injection. |
| Kind / cluster | RBAC denials, operator restart at every write boundary, forced Pod deletion without recreation, Secret rotation, API TLS verification and namespace isolation. |
| Enforced-network cluster | Direct/proxied egress deny cases including metadata, internal services, API access and fresh-Pod startup; invalidate activation on policy changes. |
| Vendor E2E | Dedicated environment, registration, dispatch, follow-up, Git clone/push, concurrent users, lost runners and fresh-order recovery. |
| Release | AMD64/ARM64 build matrix, binary integrity, no runtime updater, digest references, chart/manifests and dependency scanning. |

[Vendor E2E guidance](https://code.claude.com/docs/en/self-hosted-environments-testing) requires cloud-session
OAuth and a test-only capture path. Protect its transcripts and use synthetic prompts/repositories.
Do not add a capture hook or a transcript collector to production images.

Kill the manager before and after each status/Secret/Pod write. Pass only if a repeated order cannot
produce a second Pod creation attempt after an ambiguous outcome; accepting a missed launch is deliberate.
Also exercise Secret-owner mismatch, cancelled orders, denied API calls and expired orders after process restart.
Tests of a fake API alone cannot certify scheduling, network enforcement or registration.
