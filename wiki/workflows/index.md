---
type: Repository Documentation
title: "Contributor workflow"
description: "Research, apply, verify and release without overstating completion."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:59:38+07:00"
---

# Contributor workflow

1. Review the [active proposal](../changes/build-kubernetes-operator/proposal.md), design and delta requirements.
2. Apply ordered [tasks](../changes/build-kubernetes-operator/tasks.md) on a work branch after implementation is requested.
3. Mark tasks complete only with evidence; record remaining unknowns and tests not run.
4. Verify implementation against every scenario, then sync accepted behavior into `wiki/specs/`.
5. Archive completed change records through Code Wiki, preserving history.

Keep research links and source timestamps current when beta contracts change. Avoid claims that this independent
operator is vendor-supported. A future CI pipeline should validate wiki links, generated CRDs, Go tests,
RBAC, images and install artifacts. Do not grant cloud publishing credentials to documentation checks.
