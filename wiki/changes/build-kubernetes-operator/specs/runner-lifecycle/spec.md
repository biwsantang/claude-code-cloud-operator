---
type: Specification Delta
title: "Runner lifecycle requirements"
description: "Proposed externally observable behavior for the operator MVP."
tags: [claude-code, kubernetes, operator]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T11:01:22+07:00"
---

# Delta: runner lifecycle

## ADDED Requirements

### Requirement: Declarative fleet intent

The operator SHALL manage a namespaced `ClaudeRunnerFleet` with an external environment ID,
credential reference, pinned images, placement and resource configuration. Unsafe/cross-namespace inputs
SHALL be rejected; new Fleets SHALL start suspended. It SHALL NOT create external environments automatically.

#### Scenario: Missing prerequisite

- **GIVEN** a suspended Fleet with a missing Secret or missing valid network report
- **WHEN** its administrator requests activation
- **THEN** no polling/launch is enabled and status identifies the missing prerequisite.

### Requirement: Supported native intake

The operator SHALL retain native polling and use its documented asynchronous hook contract.
Receipts SHALL be keyed by pool/order identity, persisted with their credential before success is returned,
and frozen after acceptance. Retryable/permanent adapter failures SHALL remain distinguishable.

#### Scenario: Redelivery and partial writes

- **GIVEN** an order redelivered concurrently or after incomplete credential creation
- **WHEN** intake repeats
- **THEN** it repairs or matches one receipt/Secret pair; mismatched ownership or credentials are rejected.

### Requirement: No repeated operator submission

The controller SHALL persist a launch fence before one direct-Pod create attempt.
A missing or ambiguous Pod after consuming the fence SHALL NOT cause resubmission for that order.
It SHALL NOT claim exactly-once user-session execution.

#### Scenario: Crash after launch fence

- **GIVEN** a persisted fence and an unknown create outcome
- **WHEN** a replacement controller reconciles
- **THEN** it observes/binds an existing matching Pod or records uncertainty without another create attempt.

#### Scenario: Runner loss

- **GIVEN** a submitted Pod that exits or disappears
- **WHEN** reconcile repeats
- **THEN** that order produces no replacement; recovery requires a fresh external order.

### Requirement: Session isolation

Each session runner SHALL use a disposable Pod, non-root/read-only posture, bounded ephemeral storage,
no mounted Kubernetes token and no environment key/cloud identity. Only its own credential SHALL be mounted.
MVP inference SHALL use Anthropic API; it SHALL impose no default fleet-wide session ceiling.

#### Scenario: Session attempts infrastructure access

- **GIVEN** an activated fleet with validated enforced egress
- **WHEN** session code attempts Kubernetes, metadata, private services or public proxy bypass
- **THEN** those paths are denied and neither the environment key nor Kubernetes identity is available.

### Requirement: Retention and credential safety

Sensitive values SHALL remain outside CR spec/status, events and telemetry.
Credential deletion SHALL preserve the replay tombstone through its retention floor.
Retention expiry SHALL NOT authorize a second launch or terminate a running Pod.

#### Scenario: Replay after credential cleanup

- **GIVEN** a terminal Pod whose bearer Secret has been removed but order retention is active
- **WHEN** the order is received again
- **THEN** the stored terminal receipt prevents a new Pod.

### Requirement: Safe suspension and deletion

Suspension SHALL gate new intake/launch and preserve active execution. Deletion SHALL drain by default,
report finalizer waits and require explicit trusted abort before terminating owned active Pods.

#### Scenario: Administrator suspends a fleet

- **GIVEN** active Pods and accepted unlaunched orders
- **WHEN** the Fleet is suspended
- **THEN** active Pods continue and unlaunched orders cannot start until a still-valid order is permitted again.

### Requirement: Honest observations and verification

Status SHALL distinguish configuration, network approval, native connection, Pod state and user-session outcome.
Release acceptance SHALL include real registration, enforced-network and failure-boundary tests.

#### Scenario: Pod is Running but registration is unknown

- **GIVEN** a running Pod without documented native registration evidence
- **WHEN** status is updated
- **THEN** it reports only infrastructure state and does not assert registered or successful session execution.
