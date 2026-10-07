# Repository guidance

Read `wiki/quickstart.md` for the repository documentation map and `wiki/INSTRUCTIONS.md`
for the Code Wiki artifact contract.

This repository currently contains a proposed design only. Research and plan updates belong in
`wiki/`; implement source only when explicitly requested. Do not mark proposed behavior as shipped.

Never store work-order JWTs, environment keys, session access tokens or account emails in
CR specs/status, examples, logs, events or metrics. Use synthetic data for tests.
Keep deployment-specific cloud resources and organization metadata in downstream configuration.

Use a work branch; never commit directly to main. Verify the worktree and current branch first.
