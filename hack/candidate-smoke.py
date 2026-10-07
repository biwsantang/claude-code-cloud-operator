#!/usr/bin/env python3
"""Exercise private-candidate signature failures with ephemeral test-only keys."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--cosign", required=True)
    parser.add_argument("--evidence", required=True)
    args = parser.parse_args()
    source = Path(args.directory).resolve()
    verifier = Path(__file__).with_name("verify-candidate.py")
    env = {**os.environ, "COSIGN_PASSWORD": ""}
    checks = []

    def run(command):
        return subprocess.run(command, env=env, capture_output=True, text=True, timeout=60, check=True)

    with tempfile.TemporaryDirectory(prefix="claude-candidate-smoke-") as temporary:
        private = Path(temporary)
        candidate = private / "candidate"
        shutil.copytree(source, candidate)
        for name in ["test", "wrong"]:
            run([args.cosign, "generate-key-pair", "--output-key-prefix", str(private / name)])
        manifest = candidate / "candidate.json"
        bundle = candidate / "candidate.sigstore.json"
        run([args.cosign, "sign-blob", "--key", str(private / "test.key"), "--use-signing-config=false",
             "--tlog-upload=false", "--yes", "--bundle", str(bundle), str(manifest)])

        def verify(name, expected, public_key=None):
            result = subprocess.run(["python3", str(verifier), "--directory", str(candidate), "--cosign", args.cosign,
                                     "--public-key", str(public_key or private / "test.pub")], capture_output=True, text=True, timeout=60)
            if (result.returncode == 0) != expected:
                raise RuntimeError("candidate smoke failed: " + name + " " + result.stderr)
            checks.append({"check": name, "passed": True})

        verify("signed-inventory", True)
        verify("wrong-trusted-key", False, private / "wrong.pub")
        original = manifest.read_bytes()
        manifest.write_bytes(original + b"\n")
        verify("manifest-tamper", False)
        manifest.write_bytes(original)
        artifact = candidate / "manager-linux-amd64"
        with artifact.open("r+b") as stream:
            first = stream.read(1)
            stream.seek(0)
            stream.write(bytes([first[0] ^ 1]))
        verify("same-size-binary-tamper", False)
        with artifact.open("r+b") as stream:
            stream.write(first)
        artifact.rename(private / "saved-binary")
        verify("missing-artifact", False)
        artifact.symlink_to(private / "saved-binary")
        verify("symlink-artifact", False)
        artifact.unlink()
        (private / "saved-binary").rename(artifact)
        extra = candidate / "unlisted"
        extra.write_text("synthetic extra file")
        verify("unlisted-artifact", False)
        extra.unlink()
        shutil.copyfile(private / "test.pub", candidate / "self-published.pub")
        verify("candidate-cannot-supply-trust-key", False, candidate / "self-published.pub")
        (candidate / "self-published.pub").unlink()
        verify("restored-candidate", True)
        metadata = json.loads(original)
        boms = {path.name: len(json.loads(path.read_text())["components"]) for path in candidate.glob("*.cdx.json")}
        evidence = {"testedAt": datetime.now(timezone.utc).isoformat(), "sourceCommit": metadata["sourceCommit"],
                    "sourceTreeSHA256": metadata["sourceTreeSHA256"], "sourceDirty": metadata["sourceDirty"],
                    "candidateManifestSHA256": hashlib.sha256(original).hexdigest(), "goVersion": metadata["goVersion"],
                    "govulncheckVersion": metadata["govulncheckVersion"], "syftVersion": metadata["syftVersion"],
                    "cosignVersion": "v3.1.3", "artifactCount": len(metadata["files"]), "sbomComponentCounts": boms,
                    "ephemeralTestKeyOnly": True, "productionReleaseSigned": False, "vendorRuntimeIncluded": False,
                    "published": False, "checks": checks}
        Path(args.evidence).write_text(json.dumps(evidence, indent=2) + "\n")
    print("PASS: candidate integrity and all tamper/trust failures; ephemeral keys removed.")


if __name__ == "__main__":
    main()
