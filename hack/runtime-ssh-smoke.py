#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Exercise SSH client crypto/agent/tool paths with disposable offline credentials."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=10, **kwargs)


def main():
    record = json.loads(Path("/usr/local/share/claude-runtime/openssh-build.json").read_text())
    assert record["version"] == "10.6p1"
    assert record["signingKeyFingerprint"] == "7168B983815A5EEF59A4ADFD2A3F414E736060BA"
    assert run(["ssh", "-V"]).stderr.startswith(b"OpenSSH_10.6p1, ")
    for name, digest in record["binarySHA256"].items():
        executable = Path("/usr/bin") / name
        assert shutil.which(name) == str(executable)
        assert hashlib.sha256(executable.read_bytes()).hexdigest() == digest
    for name in record["helpers"]:
        assert not (Path("/usr/lib/openssh") / name).stat().st_mode & 0o6000
    package = subprocess.run(["dpkg-query", "-W", "-f=${db:Status-Abbrev}", "openssh-client"], capture_output=True, timeout=10)
    # dpkg can return success for a known but uninstalled package (status `un`).
    assert package.returncode != 0 or package.stdout[:2] != b"ii"
    assert not Path("/usr/sbin/sshd").exists()
    config = run(["ssh", "-G", "-o", "GSSAPIAuthentication=yes", "synthetic.invalid"]).stdout
    assert b"gssapiauthentication yes\n" in config
    assert b"securitykeyprovider internal\n" in config
    # Upstream retains GSSAPI auth, but has no Debian-specific key-exchange extension.
    extension = subprocess.run(["ssh", "-G", "-o", "GSSAPIKeyExchange=yes", "synthetic.invalid"], capture_output=True, timeout=10)
    assert extension.returncode != 0 and b"Bad configuration option" in extension.stderr
    keys = run(["ssh", "-Q", "key"]).stdout
    assert b"sk-ssh-ed25519@openssh.com" in keys and b"ssh-ed25519" in keys
    copy_id = subprocess.run(["ssh-copy-id", "-h"], capture_output=True, timeout=10)
    assert copy_id.returncode == 1 and b"Usage:" in copy_id.stdout + copy_id.stderr
    for name in ("sftp", "ssh-keyscan"):
        result = subprocess.run([name, "-h"], capture_output=True, timeout=10)
        assert result.returncode != 0 and b"usage:" in result.stderr.lower()

    with tempfile.TemporaryDirectory(prefix="ssh-sdk-") as directory:
        root = Path(directory)
        key = root / "identity"
        run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "synthetic-smoke", "-f", str(key)])
        data = root / "payload"
        data.write_bytes(b"synthetic-ssh-sdk\n")
        run(["ssh-keygen", "-Y", "sign", "-f", str(key), "-n", "file", str(data)])
        signers = root / "allowed-signers"
        signers.write_text("synthetic-smoke " + key.with_suffix(".pub").read_text())
        verify = ["ssh-keygen", "-Y", "verify", "-f", str(signers), "-I", "synthetic-smoke", "-n", "file", "-s", str(data) + ".sig"]
        run(verify, input=data.read_bytes())
        assert subprocess.run(verify, input=b"altered\n", capture_output=True, timeout=10).returncode != 0
        copied = root / "copied"
        run(["scp", str(data), str(copied)])
        assert copied.read_bytes() == data.read_bytes()
        socket = root / "agent.sock"
        agent = subprocess.Popen(["ssh-agent", "-D", "-a", str(socket)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 5
            while not socket.exists() and agent.poll() is None and time.monotonic() < deadline:
                time.sleep(0.05)
            assert socket.exists(), "agent did not start"
            environment = dict(os.environ, SSH_AUTH_SOCK=str(socket))
            run(["ssh-add", str(key)], env=environment)
            run(["ssh-add", "-T", str(key.with_suffix(".pub"))], env=environment)
            assert b"synthetic-smoke" in run(["ssh-add", "-l"], env=environment).stdout
            run(["ssh-add", "-D"], env=environment)
            assert subprocess.run(["ssh-add", "-l"], env=environment, capture_output=True, timeout=10).returncode == 1
        finally:
            agent.terminate()
            agent.wait(timeout=5)
    print("PASS: pinned SSH clients, source-bound binary hashes, offline signing/tamper rejection, scp and agent sign/remove")


if __name__ == "__main__":
    main()
