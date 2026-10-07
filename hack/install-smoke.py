#!/usr/bin/env python3
"""Exercise both install sources in an explicitly selected disposable operator kind cluster.

CNI and cert-manager must already be Ready. No vendor credential or live Fleet is created.
Keep CRDs/namespace on removal; the caller owns subsequent disposable-cluster cleanup.
"""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile
import time
from datetime import datetime, timezone


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kubeconfig", required=True)
    parser.add_argument("--context", required=True)
    parser.add_argument("--manager-image", required=True)
    parser.add_argument("--evidence", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"kind-claude-operator-[a-z0-9-]+", args.context):
        parser.error("only an explicitly named disposable operator kind cluster is allowed")
    if not re.fullmatch(r"[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}", args.manager_image):
        parser.error("provide the verified manager image digest")
    root = Path(__file__).resolve().parents[1]
    namespace = "cloud-operator-system"
    kube = ["kubectl", "--kubeconfig", str(Path(args.kubeconfig).resolve()), "--context", args.context]
    helm = ["helm", "--kubeconfig", str(Path(args.kubeconfig).resolve()), "--kube-context", args.context]

    def run(command, check=True, input=None):
        result = subprocess.run(command, cwd=root, text=True, input=input, capture_output=True, timeout=150)
        if check and result.returncode:
            raise RuntimeError(result.stderr or result.stdout)
        return result

    def get(resource, name="", ns=None):
        cmd = kube + (["-n", ns] if ns else []) + ["get", resource]
        if name:
            cmd.append(name)
        cmd += ["--ignore-not-found", "-o", "json"]
        output = run(cmd).stdout
        return json.loads(output) if output.strip() else None

    def wait(predicate, message):
        until = time.monotonic() + 30
        while not predicate():
            if time.monotonic() >= until:
                raise RuntimeError(message)
            time.sleep(1)

    nodes = get("nodes")["items"]
    prefix = args.context.removeprefix("kind-") + "-"
    if not nodes or any(not node["metadata"]["name"].startswith(prefix) for node in nodes):
        raise RuntimeError("node identity does not match the disposable test cluster")
    digest = args.manager_image.rsplit("@", 1)[1]
    for node in nodes:
        inventory = run(["docker", "exec", node["metadata"]["name"], "ctr", "-n", "k8s.io", "images", "ls"]).stdout
        records = [line.split() for line in inventory.splitlines() if line.startswith(args.manager_image + " ")]
        if len(records) != 1 or records[0][2] != digest:
            raise RuntimeError("loaded manager target does not match the requested digest; do not alias a child digest to an OCI index")
    if get("namespace", namespace) or get("validatingwebhookconfiguration", "cloud-operator-admission"):
        raise RuntimeError("test requires a fresh namespace and no existing operator installation")
    version = json.loads(run(kube + ["version", "-o", "json"]).stdout)["serverVersion"]["gitVersion"]
    run(kube + ["-n", "cert-manager", "rollout", "status", "deployment/cert-manager", "--timeout=120s"])
    labels = {"pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.33"}
    run(kube + ["apply", "-f", "-"], input=json.dumps({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": namespace, "labels": labels}}))
    checks = []

    def installed(source):
        run(kube + ["-n", namespace, "rollout", "status", "deployment/cloud-operator-controller-manager", "--timeout=120s"])
        run(kube + ["-n", namespace, "wait", "--for=condition=Ready", "certificate/cloud-operator-serving-cert", "--timeout=120s"])
        wait(lambda: run(kube + ["apply", "--dry-run=server", "-k", "config/samples"], check=False).returncode == 0, "installed fail-closed admission did not become available")
        run(kube + ["apply", "-k", "config/samples"])
        wait(lambda: get("deployment", "example-orchestrator", namespace) is not None, "suspended resources did not converge")
        fleet = get("clauderunnerfleet", "example", namespace)
        deployment = get("deployment", "example-orchestrator", namespace)
        if fleet["spec"]["suspended"] is not True or deployment["spec"]["replicas"] != 0:
            raise RuntimeError("suspended example enabled polling")
        for account, verb, resource, ns, allowed in [
            ("example-hook", "get", "secrets", namespace, False),
            ("example-hook", "create", "claudeworkorders", namespace, True),
            ("example-session", "create", "pods", namespace, False),
            ("cloud-operator-controller-manager", "get", "secrets", namespace, True),
            ("cloud-operator-controller-manager", "get", "secrets", "cert-manager", False),
        ]:
            actor = f"system:serviceaccount:{namespace}:{account}"
            result = run(kube + ["auth", "can-i", verb, resource, "--as", actor, "-n", ns], check=False)
            if result.stdout.strip() != ("yes" if allowed else "no"):
                raise RuntimeError("installed RBAC contract differs from expected permissions")
        checks.append({"source": source, "admissionDryRun": True, "suspendedReplicas": 0, "rbacPositiveAndNegative": True})

    def drain():
        run(kube + ["-n", namespace, "delete", "clauderunnerfleet", "example", "--wait=true", "--timeout=90s"])
        for resource, name in [("deployment", "example-orchestrator"), ("serviceaccount", "example-hook"), ("serviceaccount", "example-session")]:
            wait(lambda: get(resource, name, namespace) is None, "owned resources remain after suspended drain")
        checks[-1]["suspendedDrainAndGC"] = True

    chart = str(root / "charts/claude-code-cloud-operator")
    values = ["--namespace", namespace, "--set", "managerImage=" + args.manager_image, "--wait", "--timeout", "120s"]
    run(helm + ["install", "claude-operator-smoke", chart] + values)
    installed("helm")
    run(helm + ["upgrade", "claude-operator-smoke", chart] + values)
    checks[-1]["upgrade"] = True
    drain()
    run(helm + ["uninstall", "claude-operator-smoke", "--namespace", namespace, "--wait", "--timeout", "120s"])
    checks[-1]["uninstall"] = True

    source = (root / "deploy/install.yaml").read_text()
    placeholder = "example.invalid/operator@sha256:" + "0" * 64
    if source.count(placeholder) != 1:
        raise RuntimeError("raw manager placeholder changed; review the test substitution")
    source = source.replace(placeholder, args.manager_image)
    with tempfile.TemporaryDirectory(prefix="claude-install-smoke-") as directory:
        install = Path(directory) / "install.yaml"
        uninstall = Path(directory) / "uninstall.yaml"
        install.write_text(source)
        documents = source.split("---\n")
        uninstall.write_text("---\n".join(doc for doc in documents if not re.search(r"^kind: (Namespace|CustomResourceDefinition)$", doc, re.MULTILINE)))
        run(kube + ["apply", "-f", str(install)])
        installed("raw")
        run(kube + ["apply", "-f", str(install)])
        checks[-1]["upgrade"] = True
        drain()
        run(kube + ["delete", "-f", str(uninstall), "--wait=true", "--timeout=90s"])
        checks[-1]["uninstall"] = True
    for crd in ["clauderunnerfleets.runners.biwsantang.github.io", "claudeworkorders.runners.biwsantang.github.io"]:
        if get("customresourcedefinition", crd) is None:
            raise RuntimeError("uninstall deleted retained CRDs")
    if get("namespace", namespace) is None or get("deployment", "cloud-operator-controller-manager", namespace) is not None:
        raise RuntimeError("uninstall did not preserve the namespace or remove the manager")
    evidence = {"testedAt": datetime.now(timezone.utc).isoformat(), "kubernetes": version, "managerImage": args.manager_image, "loadedImageDigestVerified": True,
                "restrictedNamespace": True, "crdsAndNamespaceRetained": True, "liveVendorDispatch": False, "checks": checks}
    Path(args.evidence).write_text(json.dumps(evidence, indent=2) + "\n")
    print("PASS: Helm/raw install, update, admission, scoped RBAC, suspended drain and removal; CRDs/namespace retained")


if __name__ == "__main__":
    main()
