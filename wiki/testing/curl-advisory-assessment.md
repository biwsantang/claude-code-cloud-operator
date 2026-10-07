---
type: Repository Documentation
title: "curl advisory assessment"
description: "Distinguish runtime scanner matches, upstream claims and tested credential boundaries."
tags: [claude-code, runtime, security, curl]
status: draft
generated:
  by: code-wiki/0.1.0
  at: "2026-10-07T18:44:19+07:00"
sources:
  - resource: repo://wiki/testing/evidence/curl-runtime-smoke.json
  - resource: repo://hack/runtime-curl-smoke.py
  - resource: https://curl.se/docs/CVE-2026-9079.html
  - resource: https://curl.se/docs/CVE-2026-8926.html
  - resource: https://curl.se/docs/CVE-2026-8927.html
  - resource: https://curl.se/docs/CVE-2026-11856.html
  - resource: https://curl.se/docs/CVE-2026-8924.html
  - resource: https://curl.se/docs/CVE-2026-10536.html
  - resource: https://curl.se/docs/CVE-2026-18924.html
  - resource: https://curl.se/docs/CVE-2026-19931.html
  - resource: https://ubuntu.com/security/CVE-2026-19931
  - resource: https://github.com/curl/curl/commit/7103a93b05bc69ea98ed9d05d02fa9eeba533f2f
  - resource: https://github.com/curl/curl/commit/1c58877705b172da892cb9c18c5edb572f2d1d73
  - resource: https://curl.se/libcurl/c/CURLOPT_USERNAME.html
---

# Eight advisories, twenty-four scanner matches

The current native ARM64 and AMD64 runtime scans each retain 24 Critical matches: eight advisory IDs
against the three installed private curl packages. All are reported through Debian advisory records
with `wont-fix` state and no fixed-version bounds. There are no user ignore rules or ignored matches.
The [scan/source evidence](evidence/curl-runtime-smoke.json) binds these observations to exact images,
source files, tools and database. This page is a review assessment, not a VEX statement or an approved
exception. The automated Critical threshold still fails.

The packages contain pinned, signature-verified upstream 8.22.0, with explicit Debian-compatible
linker namespace/SONAME packaging changes. Package inventory alone cannot settle each advisory.
The table distinguishes what upstream reports from what this candidate actually demonstrates.
Severity in this table is upstream's assessment; it does not change the scanner's Critical results.

| Advisory and upstream severity | Trigger / upstream release claim | Candidate evidence and remaining verification |
| --- | --- | --- |
| [CVE-2026-9079](https://curl.se/docs/CVE-2026-9079.html), Medium | Reused libcurl handle retains explicitly cleared proxy credentials. Upstream lists 8.21.0 as fixed; the CLI is exempt. | Both old library variants reproduce credential disclosure; both new variants stop it, with a successful Basic-auth control. Native ARM64 and AMD64 execute this probe. Broader application use is not certified. |
| [CVE-2026-8926](https://curl.se/docs/CVE-2026-8926.html), Low | URL username without a password can pick another user's netrc password. Applies to CLI and library; upstream lists 8.21.0 as fixed. | Both old library variants disclose the wrong user's synthetic password; both new variants reject that reuse, with a matching-user control. Native ARM64 and AMD64 execute the library probe. A dedicated CLI netrc regression is still missing. |
| [CVE-2026-11856](https://curl.se/docs/CVE-2026-11856.html), Medium | Reusing a Digest-authenticated easy handle across HTTP origins sends the previous origin's Authorization state. CLI exempt; upstream lists 8.21.0 as fixed. | No Digest origin-switch regression has run. Require two distinct origins, valid initial Digest authentication and observation of the second origin's first request; simple Basic-auth testing does not cover it. |
| [CVE-2026-8927](https://curl.se/docs/CVE-2026-8927.html), Medium | An environment-selected proxy changes between transfers on a Digest-authenticated handle, leaking Proxy-Authorization state. CLI exempt; upstream lists 8.21.0 as fixed. | No Digest/environment-proxy switch regression has run. Explicitly clearing Basic credentials in CVE-2026-9079 does not exercise this path. |
| [CVE-2026-8924](https://curl.se/docs/CVE-2026-8924.html), Low | Trailing-dot domains bypass PSL rejection of overly broad cookies. Applies to CLI and library; upstream lists 8.21.0 as fixed. | No cookie/PSL regression has run. Require PSL-enabled binaries, a trailing-dot public-suffix cookie rejection and a legitimate host-cookie positive control. Feature listing alone is insufficient. |
| [CVE-2026-10536](https://curl.se/docs/CVE-2026-10536.html), Low | Setting HTTP/2 stream dependencies, resetting and cleaning up a handle can access freed memory. CLI exempt; upstream lists 8.21.0 as fixed by making those options no-ops. | No instrumented reproducer has run. Upstream identifies ASan/Valgrind or debug assertions as useful detectors. A normal cleanup that happens not to crash is insufficient evidence; the API behavior change also needs compatibility review. |
| [CVE-2026-18924](https://curl.se/docs/CVE-2026-18924.html), Low | Accepted HTTPS HTTP/2 server push combined with connection sharing can cause cleanup use-after-free. CLI exempt; upstream lists 8.22.0 as fixed. | No accepted-push/shared-connection reproducer or instrumented verification has run. A normal Git HTTPS fixture does not enable these conditions. |
| [CVE-2026-19931](https://curl.se/docs/CVE-2026-19931.html), Medium | Blank Negotiate credentials select an ambient identity; reusing a connection after that identity changes can authenticate as the previous user. Upstream lists 8.22.0 as unaffected. | The release claim conflicts with source/history and current application guidance. No real GSSAPI identity-switch regression has run. Keep unresolved; do not classify it from the version string. |

## Ambient identity disagreement

The [upstream advisory](https://curl.se/docs/CVE-2026-19931.html) links a single fixed-in commit.
Inspection of that [commit](https://github.com/curl/curl/commit/7103a93b05bc69ea98ed9d05d02fa9eeba533f2f)
shows a broad connection-maintenance refactor touching 15 files, rather than an isolated ambient-identity
boundary. A later [removal commit](https://github.com/curl/curl/commit/1c58877705b172da892cb9c18c5edb572f2d1d73)
deletes the special matching check and explicitly refers callers to documentation. The removed check's
identity-specific branch was guarded by `USE_WINDOWS_SSPI`; that historical Windows check is not proof
of Linux GSSAPI isolation.

[Ubuntu's security notes](https://ubuntu.com/security/CVE-2026-19931) independently describe the
add/refactor/remove sequence and state that the upstream version still has the behavior. Its distro
not-affected classification reflects equivalence to upstream. That is not a transferable clearance for
this custom runtime. Current [CURLOPT_USERNAME guidance](https://curl.se/libcurl/c/CURLOPT_USERNAME.html)
assigns applications responsibility for preventing connection reuse when an ambient identity changes.

The candidate retains GSSAPI compatibility and disposable per-session Pods. Those boundaries reduce
cross-session sharing, but an application inside one session can still change its ambient identity;
Pod isolation does not prove that application's connection reuse is safe. No global libcurl policy,
Kerberos removal, selective patch or scanner exception has been substituted for the intended SDK runtime.
The build record retains `disputedAmbientUserAdvisoryResolved: false`.

## Remaining security work

Review source and run the missing regressions against both actual library variants, with a susceptible
baseline and valid authentication/cookie controls wherever feasible. Instrumented memory tests need
instrumented libraries or Valgrind; an exit-zero production smoke cannot replace them. Keep any
application mitigation distinct from a library fix and document compatibility effects before choosing it.

A later reviewed finding decision must bind the exact source, installed binaries, image digest and
advisory evidence. Neither lowering upstream severity nor changing package names makes the current
runtime gate pass. No suppression or release approval is introduced here. Continue with the
[implementation plan](../changes/build-kubernetes-operator/tasks.md) and
[runtime review](runtime-vulnerability-review.md).
