---
type: Repository Guidance
title: "Wiki instructions"
description: "Keep observed research separate from proposed and implemented behavior."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:00:24+07:00"
---

# Wiki instructions

Maintain the OKF 0.2 Markdown wiki. The root index has only `okf_version: "0.2"` front matter;
other pages require a non-empty `type`, valid status and Code Wiki generation timestamp.

Keep researched facts in documentation and proposed behavior under `changes/`.
Reserve `specs/` for accepted current behavior after implementation and verification.
Do not sync this initial proposal into current specifications prematurely.

Cite primary sources near claims and record source resources. Never manufacture verification.
Use relative links for navigation. Keep quickstart and the root index current.
Implementation tasks stay unchecked until their behavior is implemented and tested.

Do not include proprietary code or company-specific configuration. Reuse of existing source
requires a separate ownership/licensing decision; this research only transfers general requirements.
