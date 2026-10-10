---
type: Repository Documentation
title: "Installation defaults and minimal Fleets"
description: "Shared runtime and proxy defaults with separate user Fleets and immutable accepted execution."
tags: [claude-code, operator, helm, configuration]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-10T12:00:00+07:00"
sources:
  - resource: repo://internal/contract/defaults.go
  - resource: repo://internal/controller/integration_test.go
  - resource: repo://examples/operator-values.yaml
  - resource: https://code.claude.com/docs/en/self-hosted-environments-deploy
---

# Installation defaults and minimal Fleets

This source follow-up is not part of published RC2. Helm continues to install the operator and its
CRDs, without creating user Fleets. Installation configuration supplies shared runtime and proxy
defaults; each Fleet supplies its Claude environment ID and existing credential Secret reference.

Configure the installation once using [operator values](../../examples/operator-values.yaml).
Published follow-up packaging pins both `managerImage` and the compatible `hookImage`. Source builds
must supply both. `runtime.image` names a qualified digest-pinned Claude runtime; optional
`runtime.orchestratorImage` separates the standard poller from the runner runtime. An optional Fleet
`execution.runnerImage` changes its development tools without changing the shared poller runtime.
No public Claude runtime is bundled or qualified by this change.

`network.proxy` describes an existing enforced proxy Service, namespace, Pod labels and port.
Configure it once. The chart does not deploy or qualify a proxy. A managed proxy is future work, not
an automatic substitute for CNI/network acceptance. `network.securityRevision` defaults to
`installation-v1`; increment it and revalidate after infrastructure changes.

```yaml
apiVersion: runners.biwsantang.github.io/v1alpha1
kind: ClaudeRunnerFleet
metadata:
  name: example
  namespace: cloud-operator-system
spec:
  environmentID: ccpool_REPLACE_ME
  environmentSecretRef:
    name: example-environment
```

The Secret must contain `environment-secret`. It is used only by this Fleet's pollers. Session Pods
receive their own immutable assignment credentials. Never put credentials in Helm values.

## Default values and overrides

| Setting | Default |
| --- | --- |
| Suspension | true; explicit activation is required |
| Orchestrator replicas | 2, for polling availability, not a session concurrency cap |
| Hook timeout / spawn lease | 15 / 120 seconds |
| Network report reference | `<fleet-name>-network-report`, in the Fleet namespace |
| Runner resources | CPU request 2, memory request/limit 4Gi, no CPU limit |
| Workspace | 8Gi ephemeral storage |
| Maximum assignment credential lifetime | 86400 seconds |
| Clock margin / diagnostic retention | 300 / 3600 seconds |
| Session idle / age / shutdown wait | 30 minutes / 480 minutes / 300 seconds |
| Save reminder / release push | true / false |
| Deletion policy | Drain |

Existing fully explicit Fleet manifests still take precedence and preserve their policy digests.
The [explicit example](../../config/samples/runners_v1alpha1_clauderunnerfleet-explicit.yaml) remains
available for legacy installations. Partial resource overrides inherit missing CPU/memory requests
and memory limits; an explicitly zero or invalid resource is rejected. If requesting more than 4Gi
memory, set a matching or larger memory limit. Proxy overrides are atomic: supply the complete proxy
object, or omit it. Selectors from different proxies are never combined.

Optional session settings remain on user-owned Fleets:

```yaml
spec:
  environmentID: ccpool_REPLACE_ME
  environmentSecretRef:
    name: example-environment
  execution:
    lifecycle:
      idleMinutes: 15
      maxSessionMinutes: 240
    resources:
      requests:
        cpu: "4"
```

The resolver copies configuration without writing inherited values into `spec`. Inspect
`status.effectiveConfiguration` for resolved images, execution policy and report reference, and
`status.policyDigest` for the exact effective report binding. Lifecycle defaults are frozen at new
intake; an absent lifecycle block in effective configuration uses the defaults in the table above.
Missing installation images/proxy settings cannot silently enable a Fleet.

## Network acceptance and upgrades

Keep the Fleet suspended while testing actual network enforcement. A trusted administrator supplies
the same required `report.json` evidence in the resolved report ConfigMap: actual Fleet UID,
`status.policyDigest`, successful direct/private/metadata/Kubernetes denials, proxy allow/deny tests,
fresh Pod isolation, test time and evidence. The operator does not create a passing report merely
because NetworkPolicy exists. Missing/expired/mismatched approval blocks activation and new intake.
See [installation and drain](install-and-drain.md).

Before changing installation defaults, suspend all inheriting Fleets. Upgrade the compatible CRDs and
operator, review effective status, and reproduce network acceptance whenever the resolved execution
policy changes. Explicit legacy overrides remain authoritative until the administrator removes them.
Manager/admission defaults are loaded at process start; Helm configuration changes recreate the manager, with a brief fail-closed admission outage.
Pollers use Recreate updates and carry a non-secret copy of the same defaults in their template.
If old pollers overlap a configuration upgrade, admission rejects new intake using stale defaults
with a retryable response. Cross-Fleet identity mismatches remain forbidden.

Accepted WorkOrders keep their complete execution snapshot and original digest. Completed redelivery
uses its retained receipt, even after defaults change, and never repairs a credential Secret. Already
launched Pods continue with their frozen runtime/policy. Accepted but unlaunched orders still require
current prerequisites and matching execution policy; no guarantee of launch across a policy change
is introduced. WorkOrders cannot omit images or inherit installation defaults.

For raw installation, configure manager/hook image placeholders and non-secret `FLEET_DEFAULTS` JSON
in the manager Deployment. `OPERATOR_HOOK_IMAGE` supplies the release helper fallback. The common
decoder rejects unknown fields, unpinned images and unsafe proxy identities. No new configuration CRD
or hook read permissions are added.

Resource sizing follows the current native guidance's 4Gi memory starting point while retaining the
operator's existing policy of no CPU limit. Measure representative repository builds and override
resources as needed. [Anthropic image and sizing guidance](https://code.claude.com/docs/en/self-hosted-environments-deploy#size-cpu-and-memory-for-sessions).

## Related design follow-ups

The side-conversation suggestion to call this resource `ClaudeEnvironment` matches its conceptual
mapping to an existing Claude environment. A kind rename needs a separate migration for existing
objects, receipt owner references, RBAC, admission and retained execution; this change preserves
their identities. It does not create environments in the Anthropic control plane.

Combining `/manager` and `/spawn-runner` into one project-owned OCI image could simplify publication
further. The current [Dockerfile](../../Dockerfile) builds both binaries but publishes distinct images.
Image consolidation needs coordinated release, restricted-init smoke and legacy helper-reference
compatibility changes. This defaults change supplies the existing compatible image pair automatically;
it does not claim that the manager artifact already contains the helper. Service accounts and token
mounts must remain distinct even if the artifact is unified later.
