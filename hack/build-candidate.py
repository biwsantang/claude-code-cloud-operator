#!/usr/bin/env python3
"""Build a private review bundle; this command does not sign, publish or deploy."""
import argparse
from datetime import datetime, timezone
import gzip
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True)
    parser.add_argument("--syft", required=True, help="verified Syft 1.54.1 executable")
    parser.add_argument("--govulncheck", required=True, help="govulncheck v1.8.0 built with the project Go version")
    parser.add_argument("--allow-dirty", action="store_true", help="explicitly record an uncommitted review build")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    output = Path(args.output).resolve()
    if output.is_relative_to(root):
        parser.error("output must be outside the source worktree")

    def run(command, env=None):
        return subprocess.run(command, cwd=root, env=env, text=True, capture_output=True, check=True).stdout

    def source_snapshot():
        names = run(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"]).split("\0")
        digest = hashlib.sha256()
        for name in sorted(set(names) - {""}):
            path = root / name
            if path.is_symlink() or not path.is_file():
                raise RuntimeError("source files must be regular files")
            digest.update(name.encode() + b"\0" + bytes.fromhex(sha256(path)))
        return digest.hexdigest()

    dirty = bool(run(["git", "status", "--porcelain"]).strip())
    if dirty and not args.allow_dirty:
        parser.error("source is dirty; commit it or explicitly request a dirty review build")
    commit = run(["git", "rev-parse", "HEAD"]).strip()
    snapshot = source_snapshot()
    go_version = run(["go", "env", "GOVERSION"]).strip()
    go_pin = next(line.split()[1] for line in (root / "go.mod").read_text().splitlines() if line.startswith("go "))
    if go_version != "go" + go_pin:
        parser.error("use the exact Go toolchain pinned in go.mod")
    go_binary = Path(run(["go", "env", "GOROOT"]).strip()) / "bin/go"
    # On Macs the PATH launcher may select the module toolchain automatically.
    # Keep both go build and the scanner's go subprocess on that selected compiler.
    build_env = {**os.environ, "PATH": str(go_binary.parent) + os.pathsep + os.environ["PATH"],
                 "GOTOOLCHAIN": "local", "CGO_ENABLED": "0"}
    syft_version = json.loads(run([args.syft, "version", "-o", "json"]))["version"]
    if syft_version != "1.54.1":
        parser.error("Syft version must be 1.54.1")
    scanner_version = run([args.govulncheck, "-version"], build_env)
    if "Scanner: govulncheck@v1.8.0" not in scanner_version:
        parser.error("govulncheck version must be v1.8.0")
    output.mkdir(parents=True, exist_ok=False)
    (output / "vulnerability-tool.txt").write_text(scanner_version)
    for arch in ["amd64", "arm64"]:
        env = {**build_env, "GOOS": "linux", "GOARCH": arch}
        # Text mode is deliberate: JSON/SARIF mode exits zero even with findings.
        scan = run([args.govulncheck, "./cmd", "./cmd/spawn-runner"], env)
        (output / f"vulnerability-linux-{arch}.txt").write_text(scan)
        for component, package in [("manager", "./cmd"), ("spawn-runner", "./cmd/spawn-runner")]:
            name = f"{component}-linux-{arch}"
            binary = output / name
            print("Building and cataloguing " + name, flush=True)
            run(["go", "build", "-trimpath", "-buildvcs=true", "-ldflags=-s -w", "-o", str(binary), package], env)
            build_info = run(["go", "version", "-m", str(binary)])
            (output / (name + ".buildinfo.txt")).write_text(build_info)
            run([args.syft, "scan", "file:" + str(binary), "--source-name", name, "--source-version", commit,
                 "-o", "cyclonedx-json=" + str(output / (name + ".cdx.json"))])
            bom = json.loads((output / (name + ".cdx.json")).read_text())
            if bom.get("bomFormat") != "CycloneDX" or not bom.get("components"):
                raise RuntimeError("compiled Go binary produced an empty or invalid SBOM")
            dependencies = {line.split()[1] for line in build_info.splitlines() if line.lstrip().startswith("dep\t")}
            catalogued = {component["name"] for component in bom["components"]}
            if dependencies - catalogued:
                raise RuntimeError("SBOM omits compiled Go dependencies")
    print(run([sys.executable, str(root / "hack/collect-notices.py"), "--directory", str(output),
               "--go", str(go_binary)]), end="", flush=True)
    shutil.copyfile(root / "LICENSE", output / "LICENSE")
    shutil.copyfile(root / "config/install/resources.yaml", output / "installation.yaml")
    chart = root / "charts/claude-code-cloud-operator"
    with (output / "chart.tgz").open("wb") as stream:
        with gzip.GzipFile(fileobj=stream, filename="", mode="wb", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for path in sorted(chart.rglob("*")):
                    if path.is_file():
                        info = archive.gettarinfo(str(path), "claude-code-cloud-operator/" + str(path.relative_to(chart)))
                        info.uid = info.gid = info.mtime = 0
                        info.uname = info.gname = ""
                        info.mode = 0o644
                        with path.open("rb") as source:
                            archive.addfile(info, source)
    if snapshot != source_snapshot() or run(["git", "rev-parse", "HEAD"]).strip() != commit:
        raise RuntimeError("source changed during the candidate build")
    manifest = {"schemaVersion": 1, "kind": "review-candidate", "sourceCommit": commit,
                "sourceTreeSHA256": snapshot, "sourceDirty": dirty, "goVersion": go_version,
                "builtAt": datetime.now(timezone.utc).isoformat(), "syftVersion": syft_version,
                "govulncheckVersion": "v1.8.0", "releaseApproved": False, "vendorRuntimeIncluded": False,
                "files": {path.name: {"sha256": sha256(path), "bytes": path.stat().st_size} for path in sorted(output.iterdir())}}
    (output / "candidate.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("Review candidate built. Signing, publication and acceptance remain separate.")


if __name__ == "__main__":
    main()
