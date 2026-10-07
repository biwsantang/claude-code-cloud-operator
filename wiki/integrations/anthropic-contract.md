---
type: Repository Documentation
title: "Anthropic contract research"
description: "Observed vendor behavior, Kubernetes constraints and unresolved assumptions."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T14:05:45+07:00"
sources:
  - resource: "https://code.claude.com/docs/en/self-hosted-environments"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-quickstart"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-deploy"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-configuration"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-reference"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-testing"
  - resource: "https://code.claude.com/docs/en/self-hosted-environments-identity"
  - resource: "https://kubernetes.io/docs/concepts/workloads/controllers/job/"
  - resource: "https://kubernetes.io/docs/concepts/extend-kubernetes/operator/"
  - resource: "https://book.kubebuilder.io/reference/good-practices"
  - resource: "https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/"
  - resource: "https://kubernetes.io/docs/concepts/security/secrets-good-practices/"
  - resource: "https://code.claude.com/docs/en/legal-and-compliance"
  - resource: "https://www.anthropic.com/legal/commercial-terms"
  - resource: "https://raw.githubusercontent.com/anthropics/claude-code/main/LICENSE.md"
---

# Contract research

Researched on 2026-10-07. This page records evidence; operator behavior is proposed elsewhere.

## Vendor sources inspected

| Source | Finding relevant to the design |
| --- | --- |
| [Overview](https://code.claude.com/docs/en/self-hosted-environments) | Execution moves to customer infrastructure; the control plane and session transcript remain Anthropic-hosted. Self-hosting is currently beta for Team/Enterprise organizations. |
| [Quickstart](https://code.claude.com/docs/en/self-hosted-environments-quickstart) | An organization Owner enables the feature and creates an environment. The resulting ID and environment key are installation inputs. Creating a Kubernetes resource alone does not create that external environment. |
| [Deployment](https://code.claude.com/docs/en/self-hosted-environments-deploy) | Prefer disposable isolated runners, keep broad credentials away from sessions, enforce egress and protect operator-controlled hooks/settings. Organization dispatch permissions are wider than a Kubernetes namespace boundary. |
| [Configuration](https://code.claude.com/docs/en/self-hosted-environments-configuration#on-demand-runners) | The native orchestrator invokes an asynchronous spawn hook. Its temporary work-order file must be copied before return. Order IDs deduplicate requests; one session can receive several orders. Exit codes distinguish submission, transient failures and permanent failures. |
| [Reference](https://code.claude.com/docs/en/self-hosted-environments-reference) | Hook timeout plus kill grace must fit inside the spawn lease; replica lease values must agree. HTTP success alone is insufficient for connected readiness. Short-lived runner terminal counters can disappear between scrapes. |
| [Testing](https://code.claude.com/docs/en/self-hosted-environments-testing) | Real dispatch/follow-up testing uses claude.ai OAuth. The environment key and inference API keys do not authorize session creation. Capture hooks belong only in a dedicated test environment. |
| [Identity](https://code.claude.com/docs/en/self-hosted-environments-identity) | Internal services validate the session token's prefix, ES256 signature, issuer, audience, role and expiry; routing metadata or an email is insufficient authorization. Work-order decoding is a separate concern. |

## Kubernetes sources inspected

- [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/): declarative resources plus controllers provide a Kubernetes lifecycle boundary.
- [Kubebuilder practices](https://book.kubebuilder.io/reference/good-practices): use idempotent reconciliation, separate responsibilities by Kind and standard status conditions.
- [Job behavior](https://kubernetes.io/docs/concepts/workloads/controllers/job/#handling-pod-and-container-failures): a single-completion Job can start its program twice, even with `restartPolicy: Never`. A Job is not an exactly-once execution primitive.
- [Owners and dependents](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/): namespaced owners and dependents must share a namespace.
- [Secret practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/): minimize Secret access and protect stored credentials. A CR reference does not make a credential safe from principals with Secret access.

## Independent design conclusions

Keep native polling in the vendor binary; build a Go/Kubebuilder control plane around its documented
hook. Persist sanitized order metadata separately from its credential, and use direct Pods to avoid
Job replacement behavior. This is an engineering inference, not a vendor-provided operator recipe.

Review of an existing direct-provisioner prototype highlighted failure cases to carry into tests:
concurrent delivery, partial credential creation, rotated projected tokens, expiry, malformed metadata,
unknown API outcomes and cleanup races. No prototype source is copied into this repository.

## Unknowns and implementation decisions still to validate

- Promote the tested candidate versions below into a reviewed release support matrix; exact test results do not establish universal production compatibility.
- Confirm work-order expiry/token-format fixtures against the pinned binary without logging live credentials.
- Measure hook-to-registration p99 with real scheduling and image-pull delays before choosing the lease.
- Validate suspend/drain and replica-rollout behavior against native polling, including outstanding hooks.
- Test direct-Pod registration, signal shutdown and retention on real supported clusters.
- Decide whether managed egress proxy packaging belongs in the first release or needs an external integration.
- Review vendor runtime distribution and public naming before release. Repository-owned source uses Apache-2.0; this does not license the vendor binary.

The [native spawn-hook contract](https://code.claude.com/docs/en/self-hosted-environments-configuration#the-spawn-runner-hook) supplies the poll response's HTTP Date as `CLAUDE_RUNNER_ORDER_SERVER_TIME` and asks hooks to use it for JWT expiry checks. The operator retains the signed expiry plus a bounded immutable local-minus-server offset, preserving that basis through admission, launch and partial retries. This is lifetime bookkeeping; the native runner validates registration credentials.

## Candidate pins and distribution review

| Component | Current candidate | Evidence boundary |
| --- | --- | --- |
| Repository source | Apache-2.0, root `LICENSE` and source notices | Independent implementation; no vendor binary relicensing. |
| Go / Kubebuilder | 1.27.1 / 4.16.0 | `go.mod`, `PROJECT`, cross-build CI. |
| controller-runtime / Kubernetes libraries | 0.25.2 / 0.37.1 | Locked module checks and race/API tests. |
| Kubernetes | API 1.33.0, 1.36.0, 1.37.0; installed 1.33.1, 1.36.4, 1.37.0 | Exact tested candidates; [dependency limits and evidence](../testing/implementation-evidence.md). |
| Native Claude | 2.1.285 | Signed manifest and architecture checksums; private AMD64/ARM64 runtime smoke. Real registration remains pending. |

[Anthropic's legal documentation](https://code.claude.com/docs/en/legal-and-compliance#can-customers-offer-claude-code-in-their-products)
permits product preinstallation subject to Commercial Terms and stated conditions: preserve the published
binary and authentication methods, and do not intermediate or resell end-user usage. It also restricts use
of Anthropic/Claude Code names in product or feature names. The vendor repository's
[license notice](https://raw.githubusercontent.com/anthropics/claude-code/main/LICENSE.md) reserves its rights;
[Commercial Terms](https://www.anthropic.com/legal/commercial-terms) do not implicitly grant other intellectual-property rights.

The release policy is to keep runtime builds private and publish no vendor-containing artifacts until the
intended distribution, authentication/billing model and public name are reviewed against applicable terms
or a separate agreement. The requested repository name remains unchanged and the repository remains private.
Publishing independently licensed manager/hook source does not resolve the vendor-runtime review.
Record the reviewer, applicable agreement and artifact scope before selecting release pins; task 1.2 stays pending.
