#!/usr/bin/env python3
"""Preserve upstream notice files for the exact compiled Go dependencies; no legal clearance."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile

NOTICE_NAMES = {"license", "licence", "copying", "notice", "patents", "authors", "copyright"}
MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_TOTAL_BYTES = 64 * 1024 * 1024


def notice_name(name):
    return bool(re.fullmatch(r"(?:" + "|".join(sorted(NOTICE_NAMES)) + r")(?:[._-].*)?", name.lower())
                or name.lower().endswith((".license", ".licence"))
                or re.fullmatch(r"third[._-]party[._-]notices?(?:[._-].*)?", name.lower()))


def compiled_modules(build_infos):
    """Reject replacements and bind each module version/checksum to its actual binaries."""
    modules = {}
    versions = set()
    for binary, text in sorted(build_infos.items()):
        if not text.strip():
            raise ValueError("empty compiled build information")
        versions.add(text.splitlines()[0].split()[-1])
        for line in text.splitlines()[1:]:
            fields = line.split()
            if not fields:
                continue
            if fields[0] == "=>":
                raise ValueError("replacement modules require explicit notice provenance review")
            if fields[0] != "dep":
                continue
            if len(fields) != 4 or not fields[2].startswith("v") or not fields[3].startswith("h1:"):
                raise ValueError("compiled dependency has no pinned version/checksum")
            key = (fields[1], fields[2])
            record = modules.setdefault(key, {"path": fields[1], "version": fields[2], "sum": fields[3], "binaries": []})
            if record["sum"] != fields[3]:
                raise ValueError("compiled module checksums disagree")
            record["binaries"].append(binary)
    if len(versions) != 1 or not modules:
        raise ValueError("build information must contain dependencies from one Go compiler")
    return next(iter(versions)), [modules[key] for key in sorted(modules)]


def notice_files(root):
    """Collect original bytes recursively, including nested licenses and patent/notice files."""
    root = Path(root)
    if root.is_symlink() or not root.is_dir():
        raise ValueError("notice source must be a regular directory")
    found = {}
    total = 0
    for directory, directories, files in os.walk(root, followlinks=False):
        for name in directories + files:
            if (Path(directory) / name).is_symlink():
                raise ValueError("notice source may not contain symlinks")
        for name in sorted(files):
            if not notice_name(name):
                continue
            path = Path(directory) / name
            if not path.is_file() or path.stat().st_size > MAX_FILE_BYTES:
                raise ValueError("notice file is invalid or exceeds the size bound")
            content = path.read_bytes()
            if not content or len(content) > MAX_FILE_BYTES:
                raise ValueError("notice file is empty or exceeds the size bound")
            total += len(content)
            if total > MAX_TOTAL_BYTES:
                raise ValueError("notice source exceeds the total size bound")
            found[path.relative_to(root).as_posix()] = content
    if not any(re.match(r"^(?:license|licence|copying)(?:[._-]|$)", Path(path).name.lower())
               or Path(path).name.lower().endswith((".license", ".licence")) for path in found):
        raise ValueError("source has no identifiable license file; manual review required")
    return dict(sorted(found.items()))


def write_inventory(output, sources, metadata):
    """Write normalized archive and hash index without local cache paths or interpreted licenses."""
    archive_path = output / "third-party-notices.tar.gz"
    index_path = output / "third-party-notices.json"
    if archive_path.exists() or index_path.exists():
        raise ValueError("notice outputs already exist")
    entries = []
    total = 0
    with archive_path.open("xb") as stream:
        with gzip.GzipFile(fileobj=stream, filename="", mode="wb", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for number, (identity, files) in enumerate(sources):
                    records = []
                    for relative, content in sorted(files.items()):
                        path = Path(relative)
                        if path.is_absolute() or ".." in path.parts or not relative:
                            raise ValueError("unsafe notice archive path")
                        total += len(content)
                        if total > MAX_TOTAL_BYTES:
                            raise ValueError("combined notices exceed the size bound")
                        name = f"source-{number:03d}/" + relative
                        info = tarfile.TarInfo(name)
                        info.size, info.mode, info.mtime = len(content), 0o644, 0
                        archive.addfile(info, io.BytesIO(content))
                        records.append({"sourcePath": relative, "archivePath": name, "bytes": len(content),
                                        "sha256": hashlib.sha256(content).hexdigest()})
                    entries.append({**identity, "files": records})
    index = {"schemaVersion": 1, "kind": "third-party-notice-inventory", "licenseReviewApproved": False,
             "vendorRuntimeIncluded": False, **metadata, "sources": entries,
             "archiveSHA256": hashlib.sha256(archive_path.read_bytes()).hexdigest()}
    index_path.write_text(json.dumps(index, indent=2) + "\n")
    return index


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", required=True, help="new review bundle containing compiled build information")
    parser.add_argument("--go", default="go", help="the selected compiler executable")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    output = Path(args.directory).resolve()
    if output.is_relative_to(root) or not output.is_dir():
        parser.error("candidate directory must exist outside the source worktree")
    go_binary = Path(args.go).resolve() if Path(args.go).is_file() else args.go
    env = {**os.environ, "GOTOOLCHAIN": "local"}

    def run(command):
        return subprocess.run(command, cwd=root, env=env, text=True, capture_output=True, check=True).stdout

    paths = sorted(output.glob("*.buildinfo.txt"))
    if any(path.is_symlink() for path in paths):
        parser.error("build information may not be a symlink")
    go_version, modules = compiled_modules({path.name.removesuffix(".buildinfo.txt"): path.read_text() for path in paths})
    if run([str(go_binary), "env", "GOVERSION"]).strip() != go_version:
        parser.error("notice source compiler must match the compiled binaries")
    selected = {tuple(line.split()) for line in run([str(go_binary), "list", "-m", "-f",
                "{{.Path}} {{.Version}}{{if .Replace}} REPLACED{{end}}", "all"]).splitlines()}
    if any((module["path"], module["version"]) not in selected for module in modules):
        parser.error("compiled module must match the current unreplaced build list for integrity verification")
    # Recompute extracted module/cache integrity, not merely trust download's cached Sum field.
    run([str(go_binary), "mod", "verify"])
    sources = []
    for module in modules:
        data = json.loads(run([str(go_binary), "mod", "download", "-json", module["path"] + "@" + module["version"]]))
        if data.get("Error") or (data.get("Path"), data.get("Version"), data.get("Sum")) != (module["path"], module["version"], module["sum"]):
            raise ValueError("download provenance differs from the compiled module")
        sources.append(({"kind": "module", **module}, notice_files(Path(data["Dir"]))))
    toolchain = Path(run([str(go_binary), "env", "GOROOT"]).strip())
    sources.append(({"kind": "toolchain", "version": go_version, "binaries": [path.name.removesuffix(".buildinfo.txt") for path in paths]},
                    notice_files(toolchain)))
    run([str(go_binary), "mod", "verify"])
    index = write_inventory(output, sources, {"goVersion": go_version, "moduleIntegrityVerifiedBeforeAndAfter": True,
                                            "scope": "whole compiled module trees and selected toolchain; may include uncompiled code"})
    print(f"Preserved {sum(len(source['files']) for source in index['sources'])} notice files for {len(modules)} modules and {go_version}; licensing review remains pending.")


if __name__ == "__main__":
    main()
