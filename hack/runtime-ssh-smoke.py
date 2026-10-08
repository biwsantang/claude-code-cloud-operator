#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Exercise SSH client crypto/agent/tool paths with disposable offline credentials."""

import os
from pathlib import Path
import subprocess
import tempfile
import time


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=10, **kwargs)


def main():
    assert run(["ssh", "-V"]).stderr.startswith(b"OpenSSH_")
    assert run(["dpkg-query", "-W", "-f=${db:Status-Abbrev}", "openssh-client"]).stdout[:2] == b"ii"
    assert not Path("/usr/sbin/sshd").exists()
    config = run(["ssh", "-G", "-o", "GSSAPIAuthentication=yes", "synthetic.invalid"]).stdout
    assert b"gssapiauthentication yes\n" in config
    assert b"securitykeyprovider internal\n" in config
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
    print("PASS: distro SSH clients, offline signing/tamper rejection, scp and agent sign/remove")


if __name__ == "__main__":
    main()
