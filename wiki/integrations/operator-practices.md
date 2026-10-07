---
type: Research
title: Operator practices and community lessons
description: Primary-source lessons translated into implementation and test decisions.
tags: [kubernetes, operator, research]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T13:06:00+07:00"
sources:
  - resource: https://book.kubebuilder.io/reference/good-practices
  - resource: https://ahmet.im/blog/controller-pitfalls/
  - resource: https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md
  - resource: https://github.com/kubernetes-sigs/kubebuilder/releases/tag/v4.16.0
  - resource: https://github.com/kubernetes-sigs/controller-runtime#compatibility
---

# Practices that shape this implementation

[Kubebuilder](https://book.kubebuilder.io/reference/good-practices) recommends idempotent reconciliation,
separate controllers per root kind and conditions. Fleet manages reusable infrastructure; WorkOrder owns
one irreversible submission. Status always includes observedGeneration. Unchanged reconciliation avoids writes.

[Ahmet Alp Balkan's operational lessons](https://ahmet.im/blog/controller-pitfalls/) explain several
failure patterns: stale conditions can imply false completion; implicit informers consume unexpected memory;
cached reads do not observe recent writes immediately; slow reconciliation harms large fleets.
Here, namespace caches are explicit, missing informers fail, Secret reads bypass caches, and transaction/fence
reads go directly to the API server. Reconcile handles deletion before normal work and uses bounded requeues.
No background goroutine performs a submission after its durable fence is forgotten.

[Maintainer guidance](https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md) prefers
real API-server tests over fake clients for API behavior. Unit tests cover pure builders and expiry math;
envtest covers validation, resourceVersion conflicts and concurrent intake. Cluster tests must separately
cover garbage collection, admission installation, scheduling and networking. Envtest cannot prove those.

[Kubebuilder 4.16.0 release fixes](https://github.com/kubernetes-sigs/kubebuilder/releases/tag/v4.16.0)
include webhook port/selector and envtest teardown problems. We assert one successful teardown, render
webhook service/target ports explicitly, and test installed admission before enabling polling.

## Version and licensing policy

Scaffolder: Kubebuilder 4.16.0, verified darwin_arm64 SHA-256
`69f5e207cf35193c78546b0e6dccfc42c44b24ceba81d3e22bab453ae118c328`.
Go toolchain: 1.27.1; controller-runtime: 0.25.2 with Kubernetes Go dependencies 0.37.x,
following the [maintainer compatibility matrix](https://github.com/kubernetes-sigs/controller-runtime#compatibility).
Minimum target cluster: 1.33 (CEL and admission capabilities); target versions only become supported after
installation and recovery tests pass. Library matching is distinct from cluster support.
Native Claude CLI candidate: 2.1.285, checked per architecture during runtime builds; vendor redistribution
is an unresolved release gate. Use synthetic credentials for tests. Do not publish vendor runtime artifacts
until permitted redistribution and real integration evidence are recorded.

## Validate generated children before consuming an irreversible fence

A typed CRD field is not automatically a valid Pod selector or toleration. Admission checks label keys/values,
portable Equal/Exists tolerations, namespace/reference names and an exact approved proxy Service name
before accepting execution intent. The allowed toleration shape follows [Kubernetes validation](https://github.com/kubernetes/kubernetes/blob/v1.37.1/pkg/apis/core/validation/validation.go); feature-gated numeric operators are excluded for portability to the minimum version. Service names use the older DNS label contract. Errors remain fixed and do not echo user values.

API-server tests reject malformed suspended Fleet updates before any launch, avoiding an irreversible fence
being consumed by a child Pod that the API would reject. The policy also rejects negative eviction seconds.
