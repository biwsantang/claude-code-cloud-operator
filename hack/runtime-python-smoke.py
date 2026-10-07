#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Exercise the installed Python SDK offline inside the restricted runtime smoke."""

import bz2
import ctypes
import hashlib
import json
import lzma
from pathlib import Path
import sqlite3
import ssl
import subprocess
import sys
import tempfile
import venv
import zipfile
import zlib


def main():
    assert sys.version_info[:3] == (3, 14, 8), sys.version
    assert ssl.create_default_context().cert_store_stats()["x509_ca"] > 0
    assert ctypes.CDLL(None).getpid() > 0
    data = b"synthetic-runtime-sdk" * 100
    for compress, decompress in (
        (bz2.compress, bz2.decompress),
        (lzma.compress, lzma.decompress),
        (zlib.compress, zlib.decompress),
    ):
        assert decompress(compress(data)) == data
    with sqlite3.connect(":memory:") as db:
        db.execute("CREATE TABLE sdk (value INTEGER)")
        db.execute("INSERT INTO sdk VALUES (?)", (17,))
        assert db.execute("SELECT value FROM sdk ORDER BY value").fetchone() == (17,)

    # A real wheel install tests venv/ensurepip/pip without network or build dependencies.
    with tempfile.TemporaryDirectory(prefix="python-sdk-") as directory:
        root = Path(directory)
        environment = root / "venv"
        # Match `python -m venv` on POSIX: keep the executable in /usr/local.
        # Docker tmpfs can be noexec; writable code/package data need no copied binary.
        venv.EnvBuilder(with_pip=True, symlinks=True).create(environment)
        wheel = root / "synthetic_runtime_sdk-1.0.0-py3-none-any.whl"
        info = "synthetic_runtime_sdk-1.0.0.dist-info"
        with zipfile.ZipFile(wheel, "w") as archive:
            archive.writestr("synthetic_runtime_sdk/__init__.py", "VALUE = 17\n")
            archive.writestr(f"{info}/METADATA", "Metadata-Version: 2.1\nName: synthetic-runtime-sdk\nVersion: 1.0.0\n")
            archive.writestr(f"{info}/WHEEL", "Wheel-Version: 1.0\nGenerator: runtime-smoke\nRoot-Is-Purelib: true\nTag: py3-none-any\n")
            archive.writestr(f"{info}/RECORD", "")
        python = environment / "bin/python"
        subprocess.run(
            [str(python), "-m", "pip", "--isolated", "install", "--no-index", "--no-deps", "--no-cache-dir", str(wheel)],
            check=True, timeout=30,
        )
        subprocess.run(
            [str(python), "-I", "-c", "import synthetic_runtime_sdk; assert synthetic_runtime_sdk.VALUE == 17"],
            check=True, timeout=10,
        )
    print(json.dumps({"result": "PASS", "python": sys.version.split()[0], "sqlite": sqlite3.sqlite_version,
                      "openssl": ssl.OPENSSL_VERSION, "sha256": hashlib.sha256(data).hexdigest(),
                      "checks": ["trusted-ca", "ctypes", "compression", "sqlite", "venv", "offline-pip-wheel"]}))


if __name__ == "__main__":
    main()
