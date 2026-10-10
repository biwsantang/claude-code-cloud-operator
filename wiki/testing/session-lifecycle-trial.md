---
type: Repository Documentation
title: "Dedicated native session lifecycle trial"
description: "Observed lifecycle follow-up checks, accelerated native tests and explicit remaining gates."
tags: [claude-code, lifecycle, testing]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-09T11:15:00+07:00"
sources:
  - resource: https://code.claude.com/docs/en/self-hosted-environments-testing
  - resource: repo://internal/contract/lifecycle_test.go
  - resource: repo://internal/controller/integration_test.go
  - resource: repo://internal/sessionconfig
  - resource: repo://wiki/testing/evidence/session-lifecycle-trial.json
---

# Dedicated native session lifecycle trial

This is source follow-up qualification after published `v0.1.0-rc.2`, not a new release or downstream
installation approval. The isolated kind 1.37.0 cluster used Cilium 1.20.2, cert-manager 1.21.2,
Helm 4.2.3 and native Claude 2.1.285 on ARM64. The local manager/helper/runtime digest references
and sanitized observations are in [the evidence record](evidence/session-lifecycle-trial.json).
No environment key, work-order JWT, cloud session identifier, account identity or transcript is retained there.

Use a dedicated Anthropic environment and repository integration. The test prompts create only a
synthetic uncommitted scratch file, explicitly decline committing/pushing it, and run short Bash sleeps.
The [official native testing flow](https://code.claude.com/docs/en/self-hosted-environments-testing)
provides initial `--environment --ref` dispatch and later `--cloud` messages. A test-only administrator
Stop hook captures a whitelist of reply markers and `stop_hook_active`, without exporting user content.
For reminder verification it also probes the installed helper with the same native hook input/environment.
That observation hook is not a production image feature.

The chart observations below are historical qualification of commit `10215ef`. The current chart
subsequently removed optional Fleet templating: it installs the operator only, and users apply Fleet
manifests separately. No new native trial is claimed for that packaging simplification.

## Observed checks

| Check | Evidence | Limit |
| --- | --- | --- |
| Source verification | `make verify` passed unit/race/vet/dependency checks, generation and packaging tests. | Source tests do not qualify a native runtime. |
| Actual API server | `make test-api` passed the recovery and new lifecycle compatibility/admission tests on 1.33.0 with the race detector. | Envtest has no scheduler/CNI. A root-UID rerun hit the shared Docker inotify limit before tests; the same target passed with a separate container UID/cache. |
| Curated chart | Disabled/enabled renders, alternative namespace, immutable image/input validation, lifecycle ranges and unsupported args rejection pass. Direct Helm operator-first/Fleet-second installation works. | Fresh Argo installation with the optional Fleet wave is not tested here; existing RC2 Argo trials remain historical evidence. |
| Enforced isolation | Two freshly selected Pods deny direct public/private/API/metadata/Pod Identity probes; Cilium policy-denied flows are present. Approved proxy HTTPS passes CA validation; private/IP CONNECT is rejected. | Dedicated proxy/CNI evidence, not approval of another cluster. Resource/lifecycle changes were reprobed and the report bound to the actual Fleet UID/policy. |
| Native workspace | A first and follow-up reply use the same Pod UID and observe the same scratch file. | Transcript continuity does not guarantee filesystem persistence after release. |
| Save reminder | Native Stop environment returns `block` for the dirty scratch repository; reentry has `stop_hook_active=true` and no further helper decision. Claude completes after user refusal. | No unconditional Git writes; ordinary clean/unpushed/malformed/no-origin cases are unit tested. |
| Suspend and SIGTERM | Suspend preserves the active Pod. While Bash `sleep 25` is running, normal Pod deletion observes native waiting, the completed marker and reply, then `PodExitedZero` after about 35 seconds. | Accelerated shutdown wait 30s; forced deletion, repeated signals and node loss are not save guarantees. |
| Drain | Deleting the Fleet with Drain preserves both active Pods. The short running command returns its reply and the session releases; its receipt succeeds with its launch fence retained. | During drain, infrastructure exit is recorded as `InfrastructureExitedDuringDrain`; the captured native reply supplies the completion evidence. |
| Abort | With the long Bash sleep still in flight, explicitly suspend and select Abort. Pod deletion is observed and finishes in about 34s, without the completion reply; receipt/fence and Fleet tombstones remain. | `Succeeded`/`InfrastructureExitedDuringDrain` means infrastructure cleanup, not successful user work. An unsuspended Abort patch during deletion was denied by the intake guard; the explicit suspended Abort patch succeeded. |
| Fresh native assignment | The same cloud transcript resumes under a new WorkOrder/Pod and confirms old scratch/shutdown files absent. | Ephemeral filesystem recovery is intentionally absent. |
| Native idle release | Native logs arm a 1-minute turn-end clock, report user idle/releasing, then park the session server-side. Pod is reclaimed, receipt succeeds and the launch fence remains. | About 84s after the observed reply, including native settle/release/cleanup. This is not an exact wall-clock deadline. |

The accelerated Fleet uses idle 1 minute, maximum 10 minutes, shutdown wait 30 seconds, 200m CPU and
512Mi memory requested/2Gi limit on a small node. Native startup reports a 110-second shutdown budget;
the Pod provides 150 seconds, including the already-in-flight release allowance. Product defaults remain
30/480/300 with 1 CPU, 2Gi/4Gi memory and 8Gi workspace. Full eight-hour aging, the native 15-minute
hard-cap grace and production concurrency/startup latency have not been run end to end.

Initial trials encountered old disposable sessions already queued in the dedicated environment. The
small node could not schedule all default-sized sessions, and two late-starting receipts failed. Those
failures are excluded from successful lifecycle evidence; only identified prior trial runners were stopped,
and accelerated resources were then revalidated. There is no total concurrency claim from two pollers.

Helm 4's default watcher also timed out waiting for a deliberately suspended Fleet's `Ready=False`.
The then-documented Fleet-enabled `--wait=legacy` path succeeded. A test-only imperative suspend briefly
owned an SSA field; Helm reclaimed it with `--force-conflicts` in the disposable cluster. That optional chart Fleet path has since been removed; user Fleet manifests should use one configuration owner.

## Remaining limits

Release pushing remains disabled. Read-only inspection of the authorized test repository found branch
naming/signature rules but no proved actor restriction for `claude/*`; native outcome branch names also
conflict with its current naming rule. No repository rules were changed and no push was attempted.
Builder tests cover the opt-in flag and larger grace budget; real committed-outcome push/resume remains
an acceptance gate after those repository prerequisites are supplied.

The optional runtime retains its separate vulnerability/redistribution qualification. New operator
manager/helper artifacts must pass the existing public release workflow before publishing another version.
Published RC2 artifacts were not replaced. See [lifecycle semantics](../operations/session-lifecycle.md).

After qualification the dedicated cluster and local credential/session configuration files were removed.
The unrelated existing cluster was preserved; no live Abacus environment was deployed or changed.

## Toolchain security refresh

PR verification flagged nine reachable vulnerability entries in Go 1.27.1 and the HTTP/2 dependency.
The source now requires [Go 1.27.2](https://go.dev/dl/?mode=json) and `golang.org/x/net` 0.60.0,
with its required sync/sys/term/text dependencies. Build, API-test and synthetic fixture Go images use
the verified official multi-platform digest. [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603)
and [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) document representative HTTP fixes.
The existing vulnerability and image scan gates remain enabled; no exception was added.

The live native observations and local image digests above belong to lifecycle commit `10215ef`,
before this dependency-only refresh. They are not claims about the refreshed binary digests. The refreshed `make verify` and real Kubernetes 1.33.0 API race suite pass locally. `govulncheck`
reports no vulnerabilities for either Linux AMD64 or ARM64. Architecture builds and image scans
qualify the refreshed artifacts separately in the PR checks. Native trials were
not rerun after the dedicated credentials were removed. Published RC2 binaries remain unchanged and
do not receive these fixes until a new reviewed release is published.
