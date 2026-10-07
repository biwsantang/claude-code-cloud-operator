#!/usr/bin/env python3
"""Create, fault and remove one disposable kind cluster using only synthetic credentials.

The labelled fixture image replaces native execution; no Anthropic request is made.
The synthetic report is a test assertion, NOT network conformance or rollout approval.
This fixture uses kind's default CNI. Production p99 and vendor acceptance are separate.
"""
import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kind", required=True)
    parser.add_argument("--cluster", default="claude-operator-fault")
    parser.add_argument("--manager-tag", required=True)
    parser.add_argument("--hook-tag", required=True)
    parser.add_argument("--fixture-tag", required=True)
    parser.add_argument("--evidence", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"claude-operator-fault[a-z0-9-]*", args.cluster):
        parser.error("cluster must have the dedicated claude-operator-fault prefix")
    root = Path(__file__).resolve().parents[1]
    namespace = "cloud-operator-system"
    control = args.cluster + "-control-plane"
    worker = args.cluster + "-worker"
    manager_account = f"system:serviceaccount:{namespace}:cloud-operator-controller-manager"
    node_image = "kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed"

    def run(command, check=True, input=None, timeout=180):
        result = subprocess.run(command, cwd=root, text=True, input=input, capture_output=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError(result.stderr or result.stdout or "fixture command failed")
        return result

    def stage(message):
        print(message, flush=True)

    existing = run([args.kind, "get", "clusters"]).stdout.splitlines()
    if args.cluster in existing:
        raise RuntimeError("refusing to reuse or modify an existing cluster")
    image_refs = {}
    for label, tag in [("manager", args.manager_tag), ("hook", args.hook_tag), ("fixture", args.fixture_tag)]:
        if not re.fullmatch(r"[a-z0-9]+(?:[._-][a-z0-9]+)*:[a-zA-Z0-9][a-zA-Z0-9_.-]*", tag):
            parser.error("provide a local library repository:tag")
        image = json.loads(run(["docker", "image", "inspect", tag]).stdout)[0]
        if label == "fixture" and image["Config"].get("Labels", {}).get("runners.biwsantang.github.io/fault-fixture") != "true":
            raise RuntimeError("runtime must be the synthetic fault fixture, never a vendor image")
        digest = image["Id"]
        if not re.fullmatch(r"sha256:[a-f0-9]{64}", digest) or ":" not in tag or "/" in tag:
            raise RuntimeError("use a local library image tag with its verified OCI target")
        image_refs[label] = "docker.io/library/" + tag.rsplit(":", 1)[0] + "@" + digest
    created = False
    checks = []
    records = {}
    # Colima shares the home directory, but not macOS's default /var/folders TMPDIR.
    scratch = Path.home() / ".cache" / "claude-operator-smoke"
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="claude-fault-smoke-", dir=scratch) as directory:
        temporary = Path(directory)
        policy = temporary / "audit.json"
        policy.write_text(json.dumps({"apiVersion": "audit.k8s.io/v1", "kind": "Policy", "omitStages": ["RequestReceived"],
                                     "rules": [{"level": "Metadata", "users": [manager_account], "verbs": ["create"],
                                                "namespaces": [namespace], "resources": [{"group": "", "resources": ["pods"]}]}, {"level": "None"}]}))
        run(["docker", "run", "--rm", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
             "--mount", "type=bind,src=" + str(policy) + ",dst=/audit.json,readonly", args.fixture_tag, "fixture-policy-check", "/audit.json"])
        # v1beta4 extraArgs are named entries, unlike the older kind guide's map.
        patch = """apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
apiServer:
  extraArgs:
  - name: audit-log-path
    value: /var/log/kubernetes/operator-audit.log
  - name: audit-policy-file
    value: /etc/kubernetes/policies/audit.json
  extraVolumes:
  - name: audit-policy
    hostPath: /etc/kubernetes/policies
    mountPath: /etc/kubernetes/policies
    readOnly: true
    pathType: DirectoryOrCreate
  - name: audit-log
    hostPath: /var/log/kubernetes
    mountPath: /var/log/kubernetes
    pathType: DirectoryOrCreate
"""
        config = temporary / "kind.json"
        config.write_text(json.dumps({"kind": "Cluster", "apiVersion": "kind.x-k8s.io/v1alpha4", "nodes": [
            {"role": "control-plane", "kubeadmConfigPatches": [patch], "extraMounts": [{"hostPath": str(policy), "containerPath": "/etc/kubernetes/policies/audit.json", "readOnly": True}]},
            {"role": "worker"}]}))
        kubeconfig = temporary / "kubeconfig"
        kube = ["kubectl", "--kubeconfig", str(kubeconfig), "--context", "kind-" + args.cluster]
        helm = ["helm", "--repository-config", str(temporary / "helm-repositories.yaml"), "--repository-cache", str(temporary / "helm-cache"),
                "--kubeconfig", str(kubeconfig), "--kube-context", "kind-" + args.cluster]

        def get(kind, name="", ns=namespace):
            command = kube + (["-n", ns] if ns else []) + ["get", kind]
            if name:
                command.append(name)
            output = run(command + ["--ignore-not-found", "--request-timeout=5s", "-o", "json"]).stdout
            return json.loads(output) if output.strip() else None

        def wait(predicate, message, seconds=150):
            deadline = time.monotonic() + seconds
            while not predicate():
                if time.monotonic() >= deadline:
                    raise RuntimeError(message)
                time.sleep(1)

        def apply(resource):
            run(kube + ["apply", "-f", "-"], input=json.dumps(resource))

        def patch_resource(kind, name, value):
            run(kube + ["-n", namespace, "patch", kind, name, "--type=merge", "-p", json.dumps(value)])

        def condition(obj, name, status):
            return obj is not None and any(c["type"] == name and c["status"] == status for c in obj.get("status", {}).get("conditions", []))

        def fleet_ready():
            return condition(get("clauderunnerfleet", "fault"), "Ready", "True")

        def orchestrator():
            pods = get("pods")["items"]
            ready = [p for p in pods if p["metadata"].get("labels", {}).get("runners.biwsantang.github.io/role") == "orchestrator" and
                     not p["metadata"].get("deletionTimestamp") and condition(p, "Ready", "True")]
            if len(ready) != 1:
                raise RuntimeError("one ready synthetic orchestrator is required")
            return ready[0]

        def order_name(order):
            return "order-" + hashlib.sha256(("ccpool_fault_fixture\0" + order).encode()).hexdigest()[:48]

        def intake(order, expiry, expected=0):
            for attempt in range(8):
                pod = orchestrator()
                result = run(kube + ["-n", namespace, "exec", pod["metadata"]["name"], "-c", "orchestrator", "--",
                                      "/usr/local/bin/claude", "fixture-intake", order, str(expiry)], check=False)
                if expected != 0 or result.returncode != 1:
                    break
                time.sleep(1)
            if result.returncode != expected:
                raise RuntimeError("synthetic intake returned an unexpected classification: " + result.stderr)
            return result

        def running(order, expiry):
            name = order_name(order)
            intake(order, expiry)
            wait(lambda: (get("pod", name) or {}).get("status", {}).get("phase") == "Running", "synthetic runner did not run")
            wait(lambda: (get("claudeworkorder", name) or {}).get("status", {}).get("phase") == "Running", "controller did not observe Running")
            pod = get("pod", name)
            records[order] = {"name": name, "podUID": pod["metadata"]["uid"]}
            return pod

        def same_pod(order):
            p = get("pod", records[order]["name"])
            if p is None or p["metadata"]["uid"] != records[order]["podUID"]:
                raise RuntimeError("existing synthetic execution was replaced")

        try:
            stage("Creating disposable two-node cluster with metadata-only Pod-create auditing")
            created = True
            run([args.kind, "create", "cluster", "--name", args.cluster, "--config", str(config), "--kubeconfig", str(kubeconfig), "--image", node_image, "--wait", "120s", "--retain"], timeout=300)
            nodes = get("nodes", ns=None)["items"]
            if {n["metadata"]["name"] for n in nodes} != {control, worker}:
                raise RuntimeError("node identities differ from the freshly created test cluster")
            for label, tag in [("manager", args.manager_tag), ("hook", args.hook_tag), ("fixture", args.fixture_tag)]:
                run([args.kind, "load", "docker-image", "--name", args.cluster, tag])
                for node in [control, worker]:
                    tagged = "docker.io/library/" + tag
                    run(["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "tag", "--force", tagged, image_refs[label]])
                    inventory = run(["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "ls"]).stdout
                    matches = [line.split() for line in inventory.splitlines() if line.startswith(image_refs[label] + " ")]
                    if len(matches) != 1 or matches[0][2] != image_refs[label].rsplit("@", 1)[1]:
                        raise RuntimeError("loaded OCI target differs from the requested image digest")
            stage("Installing cert-manager and operator; all runtime execution is synthetic")
            run(helm + ["repo", "add", "fault-jetstack", "https://charts.jetstack.io"])
            run(helm + ["install", "cert-manager", "fault-jetstack/cert-manager", "--namespace", "cert-manager", "--create-namespace",
                        "--version", "v1.21.2", "--set", "crds.enabled=true", "--wait", "--timeout", "150s"])
            apply({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": namespace, "labels": {"pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.33"}}})
            run(helm + ["install", "fault-operator", str(root / "charts/claude-code-cloud-operator"), "--namespace", namespace, "--set", "managerImage=" + image_refs["manager"], "--wait", "--timeout", "120s"])
            # Keep the installed controller alive while the only worker is stopped.
            patch_resource("deployment", "cloud-operator-controller-manager", {"spec": {"template": {"spec": {"nodeSelector": {"kubernetes.io/hostname": control},
                           "tolerations": [{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"}]}}}})
            run(kube + ["-n", namespace, "rollout", "status", "deployment/cloud-operator-controller-manager", "--timeout=120s"])
            run(kube + ["-n", namespace, "wait", "--for=condition=Ready", "certificate/cloud-operator-serving-cert", "--timeout=120s"])
            fleet = {"apiVersion": "runners.biwsantang.github.io/v1alpha1", "kind": "ClaudeRunnerFleet", "metadata": {"name": "fault", "namespace": namespace}, "spec": {
                "environmentID": "ccpool_fault_fixture", "environmentSecretRef": {"name": "fault-environment"}, "networkReportRef": {"name": "fault-report"},
                "orchestratorImage": image_refs["fixture"], "hookImage": image_refs["hook"], "suspended": True, "orchestratorReplicas": 1,
                "execution": {"runnerImage": image_refs["fixture"], "securityRevision": "synthetic-fault-only", "nodeSelector": {"kubernetes.io/hostname": worker},
                              "proxy": {"url": "http://proxy.proxy.svc:3128", "namespace": "proxy", "podLabels": {"app": "proxy"}, "port": 3128},
                              "resources": {"requests": {"cpu": "50m", "memory": "32Mi"}, "limits": {"memory": "64Mi"}}, "workspaceSize": "64Mi",
                              "maxTokenLifetimeSeconds": 3600, "clockMarginSeconds": 60, "diagnosticRetentionSeconds": 300}}}
            wait(lambda: run(kube + ["apply", "--dry-run=server", "-f", "-"], input=json.dumps(fleet), check=False).returncode == 0, "admission did not become ready")
            apply(fleet)
            wait(lambda: bool((get("clauderunnerfleet", "fault") or {}).get("status", {}).get("policyDigest")), "suspended Fleet did not converge")
            f = get("clauderunnerfleet", "fault")
            now = datetime.now(timezone.utc)
            report = {"fleetUID": f["metadata"]["uid"], "policyDigest": f["status"]["policyDigest"], "testedAt": now.isoformat(),
                      "validUntil": datetime.fromtimestamp(time.time() + 3600, timezone.utc).isoformat(), "evidence": "Synthetic physical fault fixture ONLY; not network conformance or rollout approval"}
            report.update({key: True for key in ["directDenied", "privateDenied", "metadataDenied", "kubernetesDenied", "proxyAllowed", "proxyDenied", "freshPodTested"]})
            apply({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "fault-report", "namespace": namespace}, "data": {"report.json": json.dumps(report)}})

            def environment(value):
                apply({"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "fault-environment", "namespace": namespace}, "stringData": {"environment-secret": value}})

            environment("synthetic-valid")
            patch_resource("clauderunnerfleet", "fault", {"spec": {"suspended": False}})
            wait(fleet_ready, "synthetic Fleet did not become ready")
            expiry = int(time.time()) + 1800
            stage("Running 16 distinct synthetic orders and replaying them without replacements")
            for i in range(16):
                running(f"batch-{i}", expiry)
                intake(f"batch-{i}", expiry)
                same_pod(f"batch-{i}")
            checks.append({"check": "physical-batch", "distinctOrders": 16, "redeliverySameUID": True})

            stage("Checking synthetic revoked/missing credentials and key-revision rollout")
            previous_orchestrator = orchestrator()["metadata"]["uid"]
            environment("synthetic-revoked")
            wait(lambda: condition(get("clauderunnerfleet", "fault"), "Connected", "False"), "revoked fixture key did not degrade connection")
            wait(lambda: get("deployment", "fault-orchestrator").get("status", {}).get("availableReplicas", 0) == 0, "revoked fixture process remained ready")
            same_pod("batch-0")
            run(kube + ["-n", namespace, "delete", "secret", "fault-environment"])
            wait(lambda: condition(get("clauderunnerfleet", "fault"), "CredentialsReady", "False") and get("deployment", "fault-orchestrator")["spec"]["replicas"] == 0, "missing key did not disable polling")
            same_pod("batch-0")
            environment("synthetic-valid")
            wait(fleet_ready, "restored fixture key did not recover")
            if orchestrator()["metadata"]["uid"] == previous_orchestrator:
                raise RuntimeError("credential revision did not replace the polling process")
            checks.append({"check": "synthetic-credential-rotation", "connectedDegraded": True, "missingKeyDisabledPolling": True, "acceptedRunnerPreserved": True})

            stage("Exhausting schedulable placement; waiting for conservative pending expiry")
            run(kube + ["cordon", worker])
            pending_expiry = int(time.time()) + 25
            intake("scheduling-exhaustion", pending_expiry)
            pending_name = order_name("scheduling-exhaustion")
            wait(lambda: condition(get("pod", pending_name), "PodScheduled", "False"), "scheduler did not report unschedulable placement")
            records["scheduling-exhaustion"] = {"name": pending_name, "podUID": get("pod", pending_name)["metadata"]["uid"]}
            wait(lambda: (get("claudeworkorder", pending_name) or {}).get("status", {}).get("terminal") is True and get("pod", pending_name) is None, "expired pending Pod was not conservatively cleaned", seconds=130)
            intake("scheduling-exhaustion", pending_expiry)
            if get("pod", pending_name) is not None or get("secret", pending_name + "-credential") is not None:
                raise RuntimeError("terminal pending replay recreated execution or credentials")
            run(kube + ["uncordon", worker])
            running("scheduling-recovery", expiry)
            checks.append({"check": "scheduling-exhaustion", "unschedulable": True, "expiryCleanup": True, "terminalReplayNoRecreation": True, "freshOrderRuns": True})

            stage("Pausing the control plane; issuing a hook request through the worker's container runtime")
            pod = orchestrator()
            container_id = next(c["containerID"].removeprefix("containerd://") for c in pod["status"]["containerStatuses"] if c["name"] == "orchestrator")
            run(["docker", "pause", control])
            try:
                unavailable = run(kube + ["get", "--raw=/readyz", "--request-timeout=3s"], check=False)
                if unavailable.returncode == 0:
                    raise RuntimeError("API remained available during the injected outage")
                result = run(["docker", "exec", worker, "crictl", "exec", container_id, "/usr/local/bin/claude", "fixture-intake", "api-outage", str(expiry)], check=False, timeout=30)
                if result.returncode != 1 or "FleetUnavailable" not in result.stderr + result.stdout:
                    raise RuntimeError("unavailable API did not produce a sanitized retryable hook result")
            finally:
                run(["docker", "unpause", control])
            wait(lambda: run(kube + ["get", "--raw=/readyz", "--request-timeout=3s"], check=False).returncode == 0, "API did not recover")
            wait(fleet_ready, "Fleet did not recover after control-plane interruption")
            for i in range(16):
                same_pod(f"batch-{i}")
            running("api-outage", expiry)
            intake("api-outage", expiry)
            same_pod("api-outage")
            checks.append({"check": "physical-api-outage", "apiUnavailable": True, "hookRetryable": True, "acceptedUIDsPreserved": True, "sameOrderRecovery": True})

            stage("Stopping and decommissioning the worker; observing orphan cleanup without replacements")
            original_node_uid = get("node", worker, ns=None)["metadata"]["uid"]
            run(["docker", "stop", worker])
            wait(lambda: condition(get("node", worker, ns=None), "Ready", "Unknown"), "stopped worker did not become Unknown", seconds=100)
            run(kube + ["delete", "node", worker, "--wait=true", "--timeout=30s"])
            wait(lambda: (get("claudeworkorder", order_name("batch-0")) or {}).get("status", {}).get("terminal") is True and get("pod", order_name("batch-0")) is None,
                 "decommissioned-node runner loss was not observed terminal", seconds=120)
            run(["docker", "start", worker])
            wait(lambda: condition(get("node", worker, ns=None), "Ready", "True"), "worker did not re-register", seconds=150)
            if get("node", worker, ns=None)["metadata"]["uid"] == original_node_uid:
                raise RuntimeError("worker decommission did not create a fresh Node identity")
            wait(fleet_ready, "polling did not recover on the re-registered node")
            intake("batch-0", expiry)
            if get("pod", order_name("batch-0")) is not None:
                raise RuntimeError("lost order was replaced after node recovery")
            running("node-recovery", expiry)
            checks.append({"check": "physical-node-loss", "stoppedNodeUnknown": True, "nodeAdministrativelyDecommissioned": True,
                           "orphanPodGC": True, "lostOrderNotReplaced": True, "freshNodeAndOrderRecover": True})

            audit = run(["docker", "exec", control, "cat", "/var/log/kubernetes/operator-audit.log"]).stdout
            events = [json.loads(line) for line in audit.splitlines() if line.strip()]
            completed = [e for e in events if e.get("stage") == "ResponseComplete" and e.get("verb") == "create" and e.get("user", {}).get("username") == manager_account and e.get("objectRef", {}).get("resource") == "pods"]
            counts = Counter(e["objectRef"]["name"] for e in completed)
            for record in records.values():
                relevant = [e for e in completed if e["objectRef"]["name"] == record["name"]]
                if counts[record["name"]] != 1 or relevant[0]["responseStatus"]["code"] != 201:
                    raise RuntimeError("metadata audit does not prove one successful Pod-create request per order")
                record["podCreateRequests"] = 1
            if any("requestObject" in e or "responseObject" in e for e in events):
                raise RuntimeError("audit policy captured bodies instead of metadata")
            checks.append({"check": "metadata-audit", "allOrdersOnePodPOST": True, "requestOrResponseBodiesCaptured": False})
            stage("Aborting owned synthetic execution; retaining the expected replay-finalizer wait")
            patch_resource("clauderunnerfleet", "fault", {"spec": {"deletionPolicy": "Abort"}})
            run(kube + ["-n", namespace, "delete", "clauderunnerfleet", "fault", "--wait=false"])
            wait(lambda: not [p for p in get("pods")["items"] if p["metadata"].get("labels", {}).get("runners.biwsantang.github.io/fleet") == "fault"], "Abort did not remove owned active execution")
            if get("clauderunnerfleet", "fault") is None:
                raise RuntimeError("Fleet bypassed active replay retention")
            checks.append({"check": "abort-retention", "ownedExecutionRemoved": True, "retentionFinalizerPreserved": True})
            version = json.loads(run(kube + ["version", "-o", "json"]).stdout)["serverVersion"]["gitVersion"]
            evidence = {"testedAt": datetime.now(timezone.utc).isoformat(), "kubernetes": version, "nodeImage": node_image, "images": image_refs,
                        "sourceBaseCommit": run(["git", "rev-parse", "HEAD"]).stdout.strip(), "fixtureSHA256": hashlib.sha256((root / "hack/fault-fixture/main.go").read_bytes()).hexdigest(),
                        "loadedImageTargetsVerified": True, "liveVendorDispatch": False, "networkConformance": False, "productionStartupP99": False,
                        "checks": checks, "orders": records}
            Path(args.evidence).write_text(json.dumps(evidence, indent=2) + "\n")
            stage("PASS: physical synthetic faults, credential rollout, replay and metadata-only one-POST audit")
        except Exception:
            if created:
                failed = run(["docker", "exec", control, "crictl", "ps", "-a", "--name", "kube-apiserver", "-q"], check=False).stdout.splitlines()
                if failed:
                    diagnostic = run(["docker", "exec", control, "crictl", "logs", "--tail=20", failed[0]], check=False)
                    stage("Synthetic test API bootstrap diagnostic:\n" + diagnostic.stdout + diagnostic.stderr)
            raise
        finally:
            if created:
                # This name was absent at entry and created only by this invocation.
                stage("Removing this invocation's disposable cluster")
                run([args.kind, "delete", "cluster", "--name", args.cluster], timeout=120)


if __name__ == "__main__":
    main()
