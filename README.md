# claude-code-cloud-operator

A namespace-scoped Kubernetes operator for Claude Code self-hosted environments.

**Status: release candidate preparation. Recovery tests and a dedicated native Claude trial pass;
production acceptance remains specific to each installation.**

The Fleet controller manages Anthropic's native orchestrator, hook identity and network policy. The native
spawn hook persists an immutable work-order receipt and credential; the WorkOrder controller creates one
disposable Pod. A durable launch fence prevents another Pod submission after a crash, ambiguous response
or deletion. This deliberately permits a missed launch and does not promise exactly-once session execution.
New Fleets start suspended. Orchestrator replicas control polling availability; there is no fleet session cap.

Session Pods are non-root, read-only, use bounded ephemeral storage and mount only their own work-order
credential. They receive no Kubernetes token. Activation requires an administrator-owned, Fleet/policy-bound network
conformance report; NetworkPolicy presence alone is insufficient. Cloud identity and environment keys
remain outside session Pods. Inference targets Anthropic API.

See the [Code Wiki map](wiki/quickstart.md), [community research](wiki/integrations/operator-practices.md),
[design](wiki/changes/build-kubernetes-operator/design.md),
[verification evidence](wiki/testing/implementation-evidence.md), and
[remaining tasks](wiki/changes/build-kubernetes-operator/tasks.md).

## Develop and verify

Go 1.27.2, Docker, Kustomize, Helm and Python 3 are used. Run from a feature worktree:

```sh
make verify
make test-api ENVTEST_VERSION=1.33.0
make test-api ENVTEST_VERSION=1.37.0
```

On macOS, the API-server suite runs inside an isolated Linux container with no published ports.
PR CI runs unit/race tests, the Kubernetes 1.33 API suite, both Go target checks and AMD64 image scans.
Main and release CI expand API coverage to 1.36/1.37 and image scans to ARM64. Generated CRDs, RBAC and DeepCopy
code come from Go markers. `make package` produces the raw installation and Helm resources from the
same Kustomize source. Do not hand-edit generated files.

Build the manager and hook with `docker build --target manager` / `--target hook`.
These images contain original dependency/toolchain notices. Version tags on reviewed `main` history run
[operator release CI](.github/workflows/release.yml): source/API/image checks, AMD64/ARM64 images in GHCR
with SBOM and BuildKit provenance, scans of the published digests, and an OCI chart plus digest-pinned
raw installation/removal manifests in a GitHub release. Public anonymous pulls gate release creation.
Published versions and immutable references are listed in [Releases](https://github.com/biwsantang/claude-code-cloud-operator/releases).
See the [release guide](wiki/workflows/release-candidates.md) for the publishing contract.

The Claude runtime is a separate image supplied by the administrator. The
[private runtime candidate](images/runtime/README.md) uses a maintained Ubuntu devcontainer base with Node/Python
and distro-packaged curl/OpenSSH. It is tested by a separate
path-filtered workflow. Its unresolved vulnerabilities block acceptance of that runtime; they do not
become findings in the manager/hook images. Neither workflow publishes Anthropic binaries.

## Install with Helm or Argo CD

The public chart is published at `oci://ghcr.io/biwsantang/charts/claude-code-cloud-operator`.
Choose a published version from [Releases](https://github.com/biwsantang/claude-code-cloud-operator/releases).
The chart supplies the released manager digest; public manager/hook images need no registry login.
Follow the [public installation guide](wiki/operations/public-installation.md) for prerequisites,
Helm commands, [Argo CD examples](examples/argocd/application.yaml), Fleet setup and upgrades.
The [curated Fleet and lifecycle controls](wiki/operations/session-lifecycle.md) are a source follow-up;
they are not included in the already published `v0.1.0-rc.2` artifacts.

For a local source build:

Use a dedicated trust namespace and an existing cert-manager installation. Build and verify an image,
then supply its digest. There is one operator installation per cluster in this initial version.

```sh
kubectl create namespace cloud-operator-system
kubectl label namespace cloud-operator-system pod-security.kubernetes.io/enforce=restricted pod-security.kubernetes.io/enforce-version=v1.33
helm install cloud-operator charts/claude-code-cloud-operator --namespace cloud-operator-system --set managerImage=REGISTRY/IMAGE@sha256:DIGEST
```

Helm leaves namespace ownership to the administrator. Raw/Kustomize installation bootstraps its namespace.
CRDs and the raw namespace carry explicit Argo prune/deletion protection.
Replace the placeholder image in `config/install` before using raw manifests. Keep Fleets suspended until
the installation, proxy/network and dedicated vendor acceptance checks pass. The chart does not create
external Claude environments, organization settings or cloud credentials.

Installing the operator does not require KMS or a release-signing key. Network reports have an optional
administrator-selected expiry; otherwise they remain valid until revoked or the Fleet/execution policy changes.
Suspend and revalidate after CNI/proxy changes, using `execution.securityRevision` to invalidate old approval.

Publisher CI owns artifact publication; installers use the trusted repository/registry and pinned
image digests. BuildKit provenance is build metadata, not an independent publisher signature.
Anthropic credentials use namespace Secret references; cert-manager manages webhook TLS. See the
[release guide](wiki/workflows/release-candidates.md).

Follow the [installation and drain guide](wiki/operations/install-and-drain.md) before an upgrade or uninstall.
Uninstalling the manager before draining orders prevents finalizer cleanup. Retained CRDs and namespace
allow recovery. Company deployment, secret-store and nodepool configuration belong downstream.

New code is Apache-2.0 licensed. Vendor software retains its own terms; its redistribution is a separate
release gate. This independent project is not endorsed by Anthropic.
