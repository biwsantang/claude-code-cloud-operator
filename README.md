# claude-code-cloud-operator

A proposed Kubernetes operator for Claude Code self-hosted environments.

**Status: research and design only. No operator, CRDs, images or installable chart are implemented yet.**

The proposed operator uses Anthropic's native orchestrator and documented spawn hook.
It manages fleet configuration and durable work-order reconciliation, while disposable
session Pods receive only their work-order credential. There is no default fleet concurrency cap.

Start with the [documentation map](wiki/quickstart.md),
[research](wiki/integrations/anthropic-contract.md), and
[implementation proposal](wiki/changes/build-kubernetes-operator/proposal.md).
The [design](wiki/changes/build-kubernetes-operator/design.md) includes resource APIs,
crash recovery, security boundaries and migration; the
[tasks](wiki/changes/build-kubernetes-operator/tasks.md) define delivery gates.

This is an independent project. Anthropic's control plane remains external.
No company-specific source code, account identifiers or deployment configuration is included.
