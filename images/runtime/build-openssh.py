#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Build pinned upstream SSH clients in a disposable stage, retaining Debian paths/config."""

import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request

VERSION = "10.6p1"
FINGERPRINT = "7168B983815A5EEF59A4ADFD2A3F414E736060BA"
ASSETS = {
    "source": (f"https://cdn.openbsd.org/pub/OpenBSD/OpenSSH/portable/openssh-{VERSION}.tar.gz", "a9dc9565dffe8640f64d863cd29a32bc4a3dbdec0566a7fc44c5d6ee767d5f39", 8 * 1024 * 1024),
    "signature": (f"https://cdn.openbsd.org/pub/OpenBSD/OpenSSH/portable/openssh-{VERSION}.tar.gz.asc", "1f8fd46ddb5ec95f5bc9e0d7b0fc34ccea3ef8f94a8e3a5d3911434cce7027cf", 16 * 1024),
    "key": ("https://cdn.openbsd.org/pub/OpenBSD/OpenSSH/RELEASE_KEY.asc", "c4a6f4692c9b8e75ec096add049fe0314b3ceff9410321f1e85907cf7a864269", 256 * 1024),
}
CLIENTS = ("ssh", "scp", "sftp", "ssh-add", "ssh-agent", "ssh-keygen", "ssh-keyscan")
HELPERS = ("ssh-keysign", "ssh-pkcs11-helper", "ssh-sk-helper")


def fetch(name, root):
    url, digest, limit = ASSETS[name]
    with urllib.request.urlopen(url, timeout=60) as response:
        if response.geturl().split(":", 1)[0] != "https":
            raise ValueError("non-HTTPS asset redirect")
        data = response.read(limit + 1)
    if len(data) > limit or hashlib.sha256(data).hexdigest() != digest:
        raise ValueError(f"invalid pinned {name} asset")
    path = root / name
    path.write_bytes(data)
    return path


def main():
    output = Path("/opt/openssh-client")
    if output.exists():
        raise ValueError("output already exists")
    with tempfile.TemporaryDirectory(prefix="openssh-build-") as directory:
        root = Path(directory)
        assets = {name: fetch(name, root) for name in ASSETS}
        key_home = root / "keys"
        key_home.mkdir(mode=0o700)
        ring = key_home / "trusted.gpg"
        subprocess.run(["gpg", "--no-options", "--no-autostart", "--homedir", str(key_home), "--batch", "--output", str(ring), "--dearmor", str(assets["key"])], check=True)
        verified = subprocess.run(["gpgv", "--homedir", str(key_home), "--keyring", str(ring), "--status-fd", "1", str(assets["signature"]), str(assets["source"])], capture_output=True, text=True, check=True)
        if not any(line.startswith(f"[GNUPG:] VALIDSIG {FINGERPRINT} ") for line in verified.stdout.splitlines()):
            raise ValueError("unexpected release signer")
        print(f"Verified OpenSSH {VERSION} source and signer {FINGERPRINT}", flush=True)
        # Verify integrity/signature before unpacking or executing upstream build code.
        with tarfile.open(assets["source"]) as archive:
            archive.extractall(root, filter="data")
        source = root / f"openssh-{VERSION}"
        configure = ["./configure", "--prefix=/usr", "--sysconfdir=/etc/ssh", "--libexecdir=/usr/lib/openssh", "--with-mantype=man", "--with-kerberos5", "--with-security-key-builtin", "--with-libedit"]
        subprocess.run(configure, cwd=source, check=True)
        subprocess.run(["make", "-j2"], cwd=source, check=True)
        # Stage installation without generating host keys; copy only client files below.
        subprocess.run(["make", "install-nosysconf", "DESTDIR=" + str(root / "installed")], cwd=source, check=True)
        output.mkdir()
        # Retain package-owned configuration, wrappers, client manuals and original notices.
        files = subprocess.check_output(["dpkg-query", "-L", "openssh-client"], text=True).splitlines()
        for item in files:
            original = Path(item)
            if not original.is_absolute() or original.is_dir() or not (original.is_file() or original.is_symlink()):
                continue
            target = output / original.relative_to("/")
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(original, target, follow_symlinks=False)
        for name in CLIENTS:
            shutil.copy2(root / "installed/usr/bin" / name, output / "usr/bin" / name)
        for name in HELPERS:
            target = output / "usr/lib/openssh" / name
            shutil.copy2(root / "installed/usr/lib/openssh" / name, target)
            target.chmod(0o755)  # No setuid helper in the restricted session image.
        # Debian's argv0 wrapper remains; upstream supplies the current copy-id script.
        shutil.copy2(source / "contrib/ssh-copy-id", output / "usr/bin/ssh-copy-id")
        (output / "usr/bin/ssh-copy-id").chmod(0o755)
        for name in CLIENTS:
            target = output / "usr/share/man/man1" / f"{name}.1"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.with_suffix(".1.gz").unlink(missing_ok=True)
            shutil.copy2(root / "installed/usr/share/man/man1" / f"{name}.1", target)
        for name in HELPERS:
            target = output / "usr/share/man/man8" / f"{name}.8"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.with_suffix(".8.gz").unlink(missing_ok=True)
            shutil.copy2(root / "installed/usr/share/man/man8" / f"{name}.8", target)
        (output / "usr/share/man/man5").mkdir(parents=True, exist_ok=True)
        shutil.copy2(root / "installed/usr/share/man/man5/ssh_config.5", output / "usr/share/man/man5/ssh_config.5")
        (output / "usr/share/man/man5/ssh_config.5.gz").unlink(missing_ok=True)
        notices = output / "usr/share/doc/openssh-client/upstream"
        notices.mkdir(parents=True)
        for name in ("LICENCE", "README", "README.md", "version.h"):
            if (source / name).is_file():
                shutil.copy2(source / name, notices / name)
        record = output / "usr/local/share/claude-runtime/openssh-build.json"
        record.parent.mkdir(parents=True)
        record.write_text(json.dumps({"version": VERSION, "signingKeyFingerprint": FINGERPRINT,
                                     "assets": {name: {"url": item[0], "sha256": item[1]} for name, item in ASSETS.items()},
                                     "configure": configure[1:], "clients": list(CLIENTS), "helpers": list(HELPERS),
                                     "setuidHelpers": False, "debianGSSAPIKeyExchangePatch": False,
                                     "binarySHA256": {name: hashlib.sha256((output / "usr/bin" / name).read_bytes()).hexdigest() for name in CLIENTS}}, indent=2) + "\n")
        assert not (output / "usr/sbin/sshd").exists()
        subprocess.run([str(output / "usr/bin/ssh"), "-V"], check=True)


if __name__ == "__main__":
    main()
