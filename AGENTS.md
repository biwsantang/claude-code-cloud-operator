# Repository guidance

Read `wiki/quickstart.md` for the repository documentation map and `wiki/INSTRUCTIONS.md`
for the Code Wiki artifact contract.

Source implementation is in progress. Research and acceptance tasks belong in `wiki/`.
Do not mark release/network/vendor acceptance as shipped without its supporting evidence.

Layout: `api/v1alpha1`, `internal/controller`, `internal/builders`, `internal/hook`, `internal/admission`,
`cmd` (manager), `cmd/spawn-runner` (hook). Generate CRDs/RBAC/DeepCopy with `make manifests generate`.
RBAC markers must be package-level and specify the installation namespace. Never hand-edit generated output.
Run `make verify` and the real API-server suite with `make test-api` before committing behavior changes.

Never store work-order JWTs, environment keys, session access tokens or account emails in
CR specs/status, examples, logs, events or metrics. Use synthetic data for tests.
Keep deployment-specific cloud resources and organization metadata in downstream configuration.

Use a work branch; never commit directly to main. Verify the worktree and current branch first.
