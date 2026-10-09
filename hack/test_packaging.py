#!/usr/bin/env python3
"""Check the generated distribution's data-retention and namespace contract."""
from pathlib import Path
import re
import subprocess
import unittest
import json
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CHART = ROOT / "charts/claude-code-cloud-operator"


def documents(source):
    return [doc[doc.index("apiVersion: "):] for doc in source.split("---\n")
            if re.search(r"^kind: ", doc, re.MULTILINE)]


def kind(doc):
    return re.search(r"^kind: (\S+)$", doc, re.MULTILINE).group(1)


class PackagingTest(unittest.TestCase):
    def test_optional_fleet_and_curated_input_validation(self):
        command = ["helm", "template", "operator-test", str(CHART), "--namespace", "operator-other"]
        values = {"managerImage":"example.invalid/manager@sha256:"+"a"*64}
        def render(values):
            with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as f:
                json.dump(values,f); f.flush()
                return subprocess.run(command+["-f",f.name], text=True, capture_output=True)
        disabled=render(values)
        self.assertEqual(disabled.returncode,0,disabled.stderr)
        self.assertNotIn("kind: ClaudeRunnerFleet",disabled.stdout)
        values["fleet"]={"enabled":True,"environmentID":"ccpool_fixture","environmentSecretRef":"existing-environment","networkReportRef":"existing-report","runtimeImage":"example.invalid/runtime@sha256:"+"b"*64,"hookImage":"example.invalid/hook@sha256:"+"c"*64,"securityRevision":"fixture","proxy":{"url":"http://proxy.proxy.svc:3128","namespace":"proxy","podLabels":{"app":"proxy"},"port":3128},"hostConfigRef":"admin-settings"}
        enabled=render(values)
        self.assertEqual(enabled.returncode,0,enabled.stderr)
        fleet=[doc for doc in documents(enabled.stdout) if kind(doc)=="ClaudeRunnerFleet"]
        self.assertEqual(len(fleet),1)
        for text in ['namespace: operator-other', 'suspended: true', 'idleMinutes: 30', 'maxSessionMinutes: 480', 'shutdownWaitSeconds: 300', 'promptToSave: true', 'pushOutcomeOnRelease: false', 'name: "admin-settings"', 'sync-wave: "20"', 'helm.sh/resource-policy: keep', 'Prune=false,Delete=false']:
            self.assertIn(text,fleet[0])
        for field,value in [("args",["--capacity","99"]),("runtimeImage","runtime:latest"),("session",{"idleMinutes":481}),("session",{"shutdownWaitSeconds":86401}),("proxy",{"podLabels":{}})]:
            bad=json.loads(json.dumps(values)); bad['fleet'][field]=value
            rejected=render(bad)
            self.assertNotEqual(rejected.returncode,0,(field,rejected.stdout))
        bad=json.loads(json.dumps(values)); bad['fleet']['environmentSecretRef']=''
        self.assertNotEqual(render(bad).returncode,0)

    def test_crds_and_namespace_retained_in_both_install_sources(self):
        raw = documents((ROOT / "deploy/install.yaml").read_text())
        crds = [doc for doc in raw if kind(doc) == "CustomResourceDefinition"]
        self.assertEqual(len(crds), 2)
        self.assertEqual(len([doc for doc in raw if kind(doc) == "Namespace"]), 1)
        for doc in raw:
            if kind(doc) in {"Namespace", "CustomResourceDefinition"}:
                self.assertIn("argocd.argoproj.io/sync-options: Prune=false,Delete=false", doc)
        chart_crds = documents((CHART / "crds/definitions.yaml").read_text())
        self.assertEqual(chart_crds, crds)

    def test_removal_cannot_delete_crds_or_namespace(self):
        raw = documents((ROOT / "deploy/install.yaml").read_text())
        removal = documents((ROOT / "deploy/uninstall.yaml").read_text())
        self.assertTrue(removal)
        self.assertEqual(removal, [doc for doc in raw if kind(doc) not in {"Namespace", "CustomResourceDefinition"}])

    def test_chart_renders_other_namespace_and_requires_digest(self):
        command = ["helm", "template", "operator-test", str(CHART), "--include-crds",
                   "--kube-version", "1.33.0", "--namespace", "operator-other"]
        missing = subprocess.run(command, text=True, capture_output=True)
        self.assertNotEqual(missing.returncode, 0)
        self.assertIn("managerImage", missing.stderr)
        rendered = subprocess.check_output(command + ["--set", "managerImage=example.invalid/manager@sha256:" + "a" * 64], text=True)
        docs = documents(rendered)
        self.assertEqual(sum(kind(doc) == "CustomResourceDefinition" for doc in docs), 2)
        self.assertFalse(any(kind(doc) == "Namespace" for doc in docs))
        self.assertNotIn("cloud-operator-system", rendered)
        self.assertIn("cloud-operator-webhook.operator-other.svc", rendered)
        self.assertIn("namespace: operator-other", rendered)
        self.assertIn("argocd.argoproj.io/sync-options: Prune=false,Delete=false", rendered)


if __name__ == "__main__":
    unittest.main()
