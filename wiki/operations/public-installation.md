---
type: Repository Documentation
title: "Public Helm and Argo CD installation"
description: "Consume a versioned public operator release with retained CRDs and a suspended Fleet."
tags: [claude-code, helm, argocd, installation]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-08T16:30:00+07:00"
sources:
  - resource: repo://.github/workflows/release.yml
  - resource: repo://examples/argocd/application.yaml
  - resource: https://helm.sh/docs/topics/registries/
  - resource: https://argo-cd.readthedocs.io/en/stable/user-guide/helm/
---

# Public installation

The public release unit is the OCI chart, manager image, hook image and raw installation/removal
manifests. No registry login, company account or AWS identity is required to fetch the operator.
Supply your own Claude self-hosted environment credential and compatible development runtime image.
The chart's default manager image is pinned to the released digest; `hook.json` in the GitHub Release
records the hook digest. The source follow-up supplies it as an installation default; RC2 still uses explicit Fleet fields. All images include Linux AMD64/ARM64.

## Prerequisites

- Kubernetes >=1.33, administrator rights for initial CRD/RBAC/webhook installation, and cert-manager
  already installed. API compatibility tests cover 1.33, 1.36 and 1.37; this is not a vendor support promise.
- One operator installation per cluster and a dedicated namespace. The manager watches only its own
  namespace. Fleets, credentials, host configuration and conformance reports must be in that namespace.
- For activation: a CNI enforcing the required network rules and a controlled proxy with a destination
  allowlist. Reproduce network conformance in your cluster; the presence of NetworkPolicy is insufficient.
- A trusted digest-pinned runtime exposing `/usr/local/bin/claude` with compatible native self-hosted
  orchestrator/session commands. It must run as UID/GID 1000 with a read-only root filesystem and use
  the writable volumes provided by the operator. Save reminders require Git and command-hook `/bin/sh`;
  legacy receipts with host configuration still require `cp`. New session configuration uses the operator's
  static helper, without a runtime jq/Python dependency. Development tools are your runtime's responsibility;
  see the [runtime candidate](../../images/runtime/README.md).

Public manager/hook images need no pull Secret. Private runtime images need a verified node credential
provider; this initial API does not expose Pod pull-secret references. Fetch credentials used by nodes
must not become cloud identity inside a session container.

## Helm

Choose a published version from [GitHub Releases](https://github.com/biwsantang/claude-code-cloud-operator/releases).
The published RC version below installs the operator. Helm's chart version omits the Git tag's `v`.

```sh
VERSION=0.1.0-rc.2
kubectl apply -f "https://raw.githubusercontent.com/biwsantang/claude-code-cloud-operator/v${VERSION}/examples/argocd/namespace.yaml"
helm install cloud-operator oci://ghcr.io/biwsantang/charts/claude-code-cloud-operator \
  --version "$VERSION" --namespace cloud-operator-system --wait --timeout 5m
kubectl -n cloud-operator-system wait --for=condition=Ready \
  certificate/cloud-operator-serving-cert --timeout=120s
```

The namespace manifest applies Restricted Pod Security labels and retention annotations. The chart
does not own the namespace or install cert-manager. Readiness must include working admission; follow
the [admission dry-run and Fleet setup](install-and-drain.md) instructions before activation.
[Helm OCI registry documentation](https://helm.sh/docs/topics/registries/)

## Argo CD

Apply the same namespace manifest and install existing cert-manager first. Copy
[the Application example](../../examples/argocd/application.yaml), select an actual released version,
and adapt the Argo namespace, project and destination. The project must allow the chart registry and
installation resource kinds. For Argo's Helm source, `repoURL` has no `oci://` prefix.

The example uses manual sync and server-side apply, omits cascading deletion, and protects the child
Application. CRDs carry `Prune=false,Delete=false`; the namespace and example Fleet have the same
protection. Keep Fleet configuration in a separate Git application targeting the installation namespace.
The chart installs the operator and CRD definitions only; user Fleet policy stays in separate manifests.
Only declare administrator-owned inputs and Fleets there. The operator owns its generated Pods,
work orders, credential Secrets and network resources.
[Argo CD Helm](https://argo-cd.readthedocs.io/en/stable/user-guide/helm/),
[Argo sync options](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-options/)

The operator repository contains generic examples. Cluster names, secret stores, team labels and
organization policy belong in the consumer's GitOps repository. Prefer annotation-based Argo tracking
and avoid copying its tracking identity into operator-created resources.

## Configure and activate a Fleet

Copy the [suspended Fleet example](../../config/samples/runners_v1alpha1_clauderunnerfleet.yaml).
For the source follow-up, configure runtime and proxy once in [installation values](../../examples/operator-values.yaml),
then set only the environment ID and Secret reference. See [inherited defaults](fleet-defaults.md).
Published RC2 still requires the [explicit Fleet](../../config/samples/runners_v1alpha1_clauderunnerfleet-explicit.yaml)
and lacks installation defaults.
Session controls belong in `spec.execution.lifecycle`; see [lifecycle defaults and behavior](session-lifecycle.md).
These lifecycle additions are source follow-up changes, absent from published `v0.1.0-rc.2` artifacts.
Create the credential Secret externally with key `environment-secret`; never commit its value or put
it in Helm values/CR specifications. After verifying certificate/admission readiness and supplying
the referenced administrator inputs, apply your reviewed manifest separately:

```sh
kubectl --context YOUR_CONTEXT apply --dry-run=server -f fleet.yaml
kubectl --context YOUR_CONTEXT apply -f fleet.yaml
```

Keep `suspended: true` while validating installation, proxy/network
enforcement and a dedicated native trial. Complete the [activation and drain guide](install-and-drain.md)
before enabling polling. An installed controller alone does not activate sessions.

## Multiple environments and credentials

One operator installation manages multiple Fleets in its watched namespace. Use one Fleet per Claude
self-hosted environment ID; adding sessions to an existing environment does not require another Fleet.
A conflicting environment claim is blocked with `Ready=False` / `PoolClaimConflict`. There is no configured
Fleet-count limit, but usable capacity depends on cluster resources and external environment capacity.

The [two-environment example](../../examples/multiple-environments/README.md) provides complete user
manifests, each with its own environment ID, Secret reference and network report. Create the environments
in Claude first, then supply their matching credentials externally. Reports bind each actual Fleet UID
and policy even when Fleets share a proxy/runtime. Infrastructure administrators own these manifests;
Claude session users do not need Kubernetes Fleet or Secret write access.

| Setting or credential | Owner / consumer |
| --- | --- |
| Helm `managerImage`, `hookImage` | Shared operator release and compatible helper |
| Helm `runtime`, `network` | Shared runtime and enforced proxy defaults |
| Fleet `execution.runnerImage` | Optional custom development runtime; explicit legacy image fields still work |
| Fleet `environmentID` and `environmentSecretRef` | That environment's polling orchestrators; Secret key is `environment-secret` |
| Single-use assignment credential | Generated WorkOrder credential Secret mounted only in its session runner |

The environment key is not mounted into disposable session runners. In orchestrator mode they use a
single-use work-order JWT, as described in the [native reference](https://code.claude.com/docs/en/self-hosted-environments-reference).
Rotating an environment Secret rolls that Fleet's polling configuration, without rolling other Fleet
pollers or changing accepted session credentials. Fleets share one infrastructure trust namespace;
multiple Fleets are not separate installations or an isolation promise for untrusted CR administrators.

## Upgrade and remove

Update versions through reviewed configuration. Suspend intake first and review compatibility with
retained orders and the supplied runtime. Argo CD applies compatible CRD changes through its desired
state; a direct Helm upgrade does not upgrade files under `crds/`. For direct Helm, pull/review the new
chart, apply its compatible CRDs with server-side apply, then upgrade the release:

```sh
NEW_VERSION=REPLACE_WITH_RELEASED_VERSION
helm pull oci://ghcr.io/biwsantang/charts/claude-code-cloud-operator \
  --version "$NEW_VERSION" --untar --untardir chart-next
kubectl apply --server-side -f chart-next/claude-code-cloud-operator/crds/definitions.yaml
helm upgrade cloud-operator oci://ghcr.io/biwsantang/charts/claude-code-cloud-operator \
  --version "$NEW_VERSION" --namespace cloud-operator-system --wait --timeout 5m
```

Do not automatically downgrade CRD schemas on rollback. Resolve suspended Fleet policy/readiness before
resuming. For removal, drain all Fleets and retained orders before removing the controller and webhook.
Then `helm uninstall cloud-operator --namespace cloud-operator-system` retains CRDs and namespace.
For raw installation or the default-namespace Argo example, the release's `uninstall.yaml` excludes CRDs
and Namespace; review it before intentional removal after drain. Custom namespaces require a matching
reviewed removal manifest. Deleting the example Argo Application alone preserves its deployed resources.
Do not delete the full `install.yaml`, force finalizers or delete the namespace as ordinary uninstall.
[Helm CRD lifecycle](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/)
