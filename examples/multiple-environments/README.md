# Two environments with one operator

Install the operator once with Helm. Then copy `fleets.yaml` into your deployment or GitOps repository.
It contains two complete, suspended user Fleet manifests in the operator namespace. Replace every
placeholder environment ID and image digest, and supply your actual proxy configuration and network
report references. New lifecycle settings require the PR #3 follow-up; published RC2 does not include them.

| Fleet | Claude environment | Existing Kubernetes Secret | Network report |
| --- | --- | --- | --- |
| team-a | ccpool_REPLACE_TEAM_A | team-a-environment | team-a-network-report |
| team-b | ccpool_REPLACE_TEAM_B | team-b-environment | team-b-network-report |

Each Secret must contain `environment-secret` for its corresponding Claude environment. Create Secrets
through your secret-management system or from protected local files. These example command shapes
create the inputs without putting credential values in manifests or command arguments:

```sh
kubectl --context YOUR_CONTEXT -n cloud-operator-system create secret generic team-a-environment \
  --from-file=environment-secret=/secure/team-a-environment-key
kubectl --context YOUR_CONTEXT -n cloud-operator-system create secret generic team-b-environment \
  --from-file=environment-secret=/secure/team-b-environment-key
```

After operator certificate/admission readiness and supplying the referenced inputs, apply your reviewed
manifest with your actual context:

```sh
kubectl --context YOUR_CONTEXT apply --dry-run=server -f fleets.yaml
kubectl --context YOUR_CONTEXT apply -f fleets.yaml
```

Keep both Fleets suspended while reproducing network conformance and binding each report to its actual
Fleet UID and declared policy digest. Then activate each independently through its manifest owner.
See [public installation](../../wiki/operations/public-installation.md) and
[installation and drain](../../wiki/operations/install-and-drain.md) for the complete procedure.

`managerImage` belongs to the shared operator Helm release. The Fleet owns `orchestratorImage`,
`hookImage` and `execution.runnerImage`, plus its resources and lifecycle. Its polling replicas use
that Fleet's environment Secret. Each disposable session runner receives only its own single-use
assignment credential; the environment key is not mounted in that runner. Rotating one environment
key changes its pollers without rolling another Fleet's pollers.

Use one Fleet per Claude environment ID in the watched namespace. A duplicate environment claim is
blocked with `Ready=False` and reason `PoolClaimConflict`; it does not create a second polling deployment.
You can add more distinct environments by adding manifests. There is no configured Fleet-count limit;
cluster resources still bound usable capacity. Polling replicas do not impose a total session cap.
Fleets share the administrator trust namespace and are not separate operator installations or a claim
of isolation between untrusted infrastructure administrators. Session users use Claude permissions;
they do not need permission to edit Fleet manifests or Secrets.

The proxy and tested runtime can be shared where your policy permits, but each environment has its own
credential and each network approval binds its own Fleet UID/policy. Suspend and drain each Fleet before
removing the shared operator. These examples do not create external Claude environments or valid proofs.
