#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Retain private image inventory/scan evidence and enforce the Critical threshold."""

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import urllib.request

TOOLS = {
    "grype": ("0.120.1", "https://github.com/anchore/grype/releases/download/v0.120.1/grype_0.120.1_linux_amd64.tar.gz", "0a9ee97ef5ae2ee953b0a80098105052e846cdbe319a57d808b519c33cd1343d"),
    "syft": ("1.54.1", "https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_linux_amd64.tar.gz", "c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47"),
}


def install(name, root):
    if os.uname().sysname != "Linux" or os.uname().machine != "x86_64":
        raise ValueError("automatic scanner installation requires Linux AMD64; supply verified tools elsewhere")
    _, url, digest = TOOLS[name]
    with urllib.request.urlopen(url, timeout=60) as response:
        if response.geturl().split(":", 1)[0] != "https":
            raise ValueError("non-HTTPS tool redirect")
        data = response.read(128 * 1024 * 1024 + 1)
    if len(data) > 128 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != digest:
        raise ValueError("invalid scanner archive")
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        member = archive.getmember(name)
        if not member.isfile() or member.size > 128 * 1024 * 1024:
            raise ValueError("invalid scanner executable")
        executable = root / name
        executable.write_bytes(archive.extractfile(member).read())
        executable.chmod(0o700)
    return executable


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--grype", type=Path)
    parser.add_argument("--syft", type=Path)
    parser.add_argument("--database-cache", type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    sha = lambda path: hashlib.sha256(Path(path).read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix="runtime-scan-") as directory:
        root = Path(directory)
        # Scanners have no reason to inherit CI/cloud/account credential environment variables.
        environment = {k: v for k, v in os.environ.items() if k in {"PATH", "TMPDIR", "DOCKER_HOST", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR"}}
        environment["HOME"] = str(root)
        if "DOCKER_HOST" not in environment:
            context = json.loads(subprocess.check_output(["docker", "context", "inspect"]))[0]
            environment["DOCKER_HOST"] = context["Endpoints"]["docker"]["Host"]
        executables = {name: (getattr(args, name) or install(name, root)).resolve() for name in TOOLS}
        versions = {name: json.loads(subprocess.check_output([str(path), "version", "-o", "json"], env=environment))["version"] for name, path in executables.items()}
        assert all(versions[name] == TOOLS[name][0] for name in TOOLS), versions
        config = {"check-for-app-update": False, "ignore": [], "only-fixed": False, "only-notfixed": False,
                  "db": {"cache-dir": str(args.database_cache or root / "database"), "auto-update": args.database_cache is None,
                         "validate-age": True, "max-allowed-built-age": "120h", "require-update-check": True}}
        config_path = root / "grype-config.json"
        config_path.write_text(json.dumps(config))
        for name, command in [
            ("sbom", [str(executables["syft"]), "docker:" + args.image, "-o", "syft-json"]),
            ("grype", [str(executables["grype"]), "docker:" + args.image, "--config", str(config_path), "--scope", "squashed", "--fail-on", "critical", "-o", "json"]),
        ]:
            with (args.output / (name + ".json")).open("w") as output, (args.output / (name + ".stderr")).open("w") as errors:
                result = subprocess.run(command, env=environment, stdout=output, stderr=errors)
            if result.returncode not in ([0, 2] if name == "grype" else [0]):
                raise ValueError(f"{name} scan failed with exit {result.returncode}")
        report = json.loads((args.output / "grype.json").read_text())
        sbom = json.loads((args.output / "sbom.json").read_text())
        db = report["descriptor"]["db"]["status"]
        assert db["valid"] is True
        counts = Counter(item["vulnerability"]["severity"] for item in report["matches"])
        summary = {"recordedAt": datetime.now(timezone.utc).isoformat(),
                   "sourceCommit": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
                   "sourceDirty": bool(subprocess.check_output(["git", "status", "--porcelain"])),
                   "dockerImage": json.loads(subprocess.check_output(["docker", "image", "inspect", args.image]))[0],
                   "imageConfigID": report["source"]["target"]["imageID"],
                   "tools": {name: {"version": versions[name], "executableSHA256": sha(path), "archiveSHA256": TOOLS[name][2] if not getattr(args, name) else None} for name, path in executables.items()},
                   "databaseStatus": {k: db[k] for k in ["schemaVersion", "from", "built", "valid"]},
                   "scope": "squashed", "userIgnoreRules": [], "ignoredMatchCount": len(report.get("ignoredMatches", [])),
                   "effectiveDefaultKernelHeaderRules": [rule["package"] for rule in report["descriptor"]["configuration"]["ignore"]],
                   "matchesBySeverity": {name: counts[name] for name in ["Critical", "High", "Medium", "Low", "Negligible", "Unknown"]},
                   "criticalGateExitCode": result.returncode, "criticalGatePassed": result.returncode == 0,
                   "sbomArtifactCount": len(sbom["artifacts"]), "rawReportSHA256": sha(args.output / "grype.json"), "rawSBOMSHA256": sha(args.output / "sbom.json"),
                   "releaseApproved": False, "vendorAcceptancePassed": False, "published": False}
        # Full Docker metadata is private review evidence, never printed into the job log.
        (args.output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps({"matchesBySeverity": summary["matchesBySeverity"], "ignoredMatchCount": summary["ignoredMatchCount"],
                          "criticalGateExitCode": result.returncode, "rawReportSHA256": summary["rawReportSHA256"], "rawSBOMSHA256": summary["rawSBOMSHA256"]}))
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
