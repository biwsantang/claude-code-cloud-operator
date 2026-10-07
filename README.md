# claude-code-cloud-operator

A namespace-scoped Kubernetes operator for Claude Code self-hosted environments.

**Status: implementation candidate. Local recovery tests pass; production acceptance and vendor E2E remain pending.**

The Fleet controller manages Anthropic's native orchestrator, hook identity and network policy. The native
spawn hook persists an immutable work-order receipt and credential; the WorkOrder controller creates one
disposable Pod. A durable launch fence prevents another Pod submission after a crash, ambiguous response
or deletion. This deliberately permits a missed launch and does not promise exactly-once session execution.
New Fleets start suspended. Orchestrator replicas control polling availability; there is no fleet session cap.

Session Pods are non-root, read-only, use bounded ephemeral storage and mount only their own work-order
credential. They receive no Kubernetes token. Activation requires a fresh administrator-owned network
conformance report; NetworkPolicy presence alone is insufficient. Cloud identity and environment keys
remain outside session Pods. Inference targets Anthropic API.

See the [Code Wiki map](wiki/quickstart.md), [community research](wiki/integrations/operator-practices.md),
[design](wiki/changes/build-kubernetes-operator/design.md),
[verification evidence](wiki/testing/implementation-evidence.md), and
[remaining tasks](wiki/changes/build-kubernetes-operator/tasks.md).

## Develop and verify

Go 1.27.1, Docker, Kustomize, Helm and Python 3 are used. Run from a feature worktree:

```sh
make verify
make test-api ENVTEST_VERSION=1.33.0
make test-api ENVTEST_VERSION=1.37.0
```

On macOS, the API-server suite runs inside an isolated Linux container with no published ports.
CI also runs unit/race/API tests and cross-builds Linux AMD64/ARM64. Generated CRDs, RBAC and DeepCopy
code come from Go markers. `make package` produces the raw installation and Helm resources from the
same Kustomize source. Do not hand-edit generated files.

Build the manager and hook with `docker build --target manager` / `--target hook`. The
[vendor runtime candidate](images/runtime/README.md) is a separate private build with signed manifest
verification. No runtime images have been published by this repository.

## Installation candidate

Use a dedicated trust namespace and an existing cert-manager installation. Build and verify an image,
then supply its digest. There is one operator installation per cluster in this initial version.

```sh
kubectl create namespace cloud-operator-system
kubectl label namespace cloud-operator-system pod-security.kubernetes.io/enforce=restricted pod-security.kubernetes.io/enforce-version=v1.33
helm install cloud-operator charts/claude-code-cloud-operator --namespace cloud-operator-system --set managerImage=REGISTRY/IMAGE@sha256:DIGEST
```

Helm leaves namespace ownership to the administrator. Raw/Kustomize installation bootstraps its namespace.
Replace the placeholder image in `config/install` before using raw manifests. Keep Fleets suspended until
the installation, proxy/network and dedicated vendor acceptance checks pass. The chart does not create
external Claude environments, organization settings or cloud credentials.

Installing the operator does not require KMS or a release-signing key. Publisher CI owns release
signing; installers verify its trusted identity and use pinned images. Anthropic credentials are
supplied through namespace Secret references, while cert-manager manages webhook TLS. See the
[signing responsibility guide](wiki/workflows/release-candidates.md).

Follow the [installation and drain guide](wiki/operations/install-and-drain.md) before an upgrade or uninstall.
Uninstalling the manager before draining orders prevents finalizer cleanup. Retained CRDs and namespace
allow recovery. Company deployment, secret-store and nodepool configuration belong downstream.

New code is Apache-2.0 licensed. Vendor software retains its own terms; its redistribution is a separate
release gate. This independent project is not endorsed by Anthropic.
