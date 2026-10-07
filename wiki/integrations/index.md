---
type: Repository Documentation
title: "Integrations"
description: "Primary-source research for the supported Claude runner boundary."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:02:48+07:00"
---

# Integrations

[Anthropic contract research](anthropic-contract.md) records the seven supplied documentation pages,
Kubernetes semantics and decisions that follow from them. Sources were inspected on 2026-10-07
in Asia/Bangkok; beta contracts must be checked again when implementing.

The MVP targets Anthropic API inference. Cloud IAM, external secret stores, organization admin
operations and private service authorization remain downstream responsibilities.

[Existing operator alternatives](existing-projects.md) compares primary upstream documentation and defines
the adoption gate before a new controller is scaffolded.
