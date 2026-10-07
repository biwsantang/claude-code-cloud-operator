#!/usr/bin/env python3
"""Verify an offline signature and every private candidate file; no release approval."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess


def verify_inventory(root, manifest):
    if manifest.get("schemaVersion") != 1 or manifest.get("kind") != "review-candidate":
        raise ValueError("unknown candidate schema")
    if not re.fullmatch(r"[a-f0-9]{40}", manifest.get("sourceCommit", "")):
        raise ValueError("invalid source commit")
    inventory = manifest.get("files")
    if not isinstance(inventory, dict) or not inventory:
        raise ValueError("empty artifact inventory")
    actual = set()
    for path in root.rglob("*"):
        if path.is_symlink():
            raise ValueError("symlinks are not candidate artifacts")
        if path.is_file():
            actual.add(str(path.relative_to(root)))
    if actual - {"candidate.json", "candidate.sigstore.json"} != set(inventory):
        raise ValueError("missing or unlisted artifact")
    for name, expected in inventory.items():
        if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,254}", name) or name in {"candidate.json", "candidate.sigstore.json"}:
            raise ValueError("unsafe artifact filename")
        if not isinstance(expected, dict) or not re.fullmatch(r"[a-f0-9]{64}", expected.get("sha256", "")) or type(expected.get("bytes")) is not int or expected["bytes"] < 0:
            raise ValueError("invalid artifact digest or size")
        path = root / name
        digest = hashlib.sha256()
        with path.open("rb") as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(chunk)
        if path.stat().st_size != expected["bytes"] or digest.hexdigest() != expected["sha256"]:
            raise ValueError("artifact content does not match the signed inventory")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--cosign", required=True)
    parser.add_argument("--public-key", required=True, help="trusted key obtained independently of the candidate")
    args = parser.parse_args()
    root = Path(args.directory).resolve()
    public_key = Path(args.public_key).resolve()
    if public_key.is_relative_to(root):
        parser.error("trusted public key must come from outside the candidate directory")
    manifest = root / "candidate.json"
    bundle = root / "candidate.sigstore.json"
    if manifest.is_symlink() or bundle.is_symlink() or not manifest.is_file() or not bundle.is_file() or manifest.stat().st_size > 1024 * 1024 or bundle.stat().st_size > 1024 * 1024:
        parser.error("missing or unsafe signed manifest/bundle")
    version = subprocess.run([args.cosign, "version", "--json"], capture_output=True, text=True, check=True)
    if json.loads(version.stdout).get("gitVersion") != "v3.1.3":
        parser.error("Cosign version must be v3.1.3")
    # Private-key policy has no public transparency-log evidence. No OIDC/certificate fallback.
    result = subprocess.run([args.cosign, "verify-blob", "--key", str(public_key), "--bundle", str(bundle),
                             "--insecure-ignore-tlog", str(manifest)], capture_output=True, text=True, timeout=60)
    if result.returncode:
        parser.error("candidate signature does not verify with the independently trusted key")
    try:
        verify_inventory(root, json.loads(manifest.read_text()))
    except (ValueError, OSError, TypeError) as error:
        parser.error(str(error))
    print("PASS: signature and every artifact match. This proves integrity, not release or deployment approval.")


if __name__ == "__main__":
    main()
