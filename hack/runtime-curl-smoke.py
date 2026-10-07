#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Exercise actual libcurl credential boundaries and Git HTTPS in an offline container."""

import base64
import ctypes
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
import ssl
import subprocess
import sys
import tempfile
import threading
from urllib.parse import urlsplit


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=15, **kwargs)


class HTTPFixture(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def do_GET(self):
        self.server.observed.append({"path": self.path, "authorization": self.headers.get("Authorization"),
                                     "proxyAuthorization": self.headers.get("Proxy-Authorization"),
                                     "host": self.headers.get("Host"), "cookie": self.headers.get("Cookie")})
        self.send_response(200)
        body = b"synthetic-curl-sdk\n"
        self.send_header("Content-Length", str(len(body)))
        if self.path == "/set-cookie":
            self.send_header("Set-Cookie", "super=synthetic; Domain=co.uk.; Path=/")
            self.send_header("Set-Cookie", "legitimate=synthetic; Domain=example.co.uk.; Path=/")
        self.end_headers()
        self.wfile.write(body)


class DigestFixture(BaseHTTPRequestHandler):
    """Validate the synthetic Digest control before testing state isolation."""
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def do_GET(self):
        proxy = self.server.is_proxy
        header = self.headers.get("Proxy-Authorization" if proxy else "Authorization")
        self.server.observed.append(header)
        status = 407 if proxy else 401
        valid = False
        if header and header.startswith("Digest "):
            fields = {key: quoted or bare for key, quoted, bare in
                      re.findall(r'(\w+)=(?:"([^"]*)"|([^,\s]+))', header[7:])}
            # MD5 is the deliberately selected Digest protocol, not a security hash for artifacts.
            def digest(value):
                return hashlib.md5(value.encode(), usedforsecurity=False).hexdigest()
            username = "synthetic-proxy" if proxy else "synthetic-user"
            required = {"username": username, "realm": "synthetic-realm", "nonce": "synthetic-nonce",
                        "qop": "auth"}
            valid = all(fields.get(key) == value for key, value in required.items())
            target = urlsplit(self.path)
            origin_form = target.path + ("?" + target.query if target.query else "")
            valid = valid and fields.get("uri") in {self.path, origin_form}
            if valid:
                a1 = digest(f"{username}:synthetic-realm:synthetic-password")
                a2 = digest(f"GET:{fields['uri']}")
                expected = digest(f"{a1}:synthetic-nonce:{fields.get('nc')}:{fields.get('cnonce')}:auth:{a2}")
                valid = fields.get("response") == expected and bool(fields.get("cnonce"))
            status = 200 if valid else 403
        if valid:
            self.server.authenticated += 1
        self.send_response(status)
        if not header:
            self.send_header("Proxy-Authenticate" if proxy else "WWW-Authenticate",
                             'Digest realm="synthetic-realm", nonce="synthetic-nonce", algorithm=MD5, qop="auth"')
        self.send_header("Content-Length", "0")
        self.end_headers()


class GitFixture(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def respond(self):
        url = urlsplit(self.path)
        length = int(self.headers.get("Content-Length", "0"))
        assert 0 <= length <= 1024 * 1024
        env = dict(os.environ, GIT_PROJECT_ROOT=str(self.server.project_root), GIT_HTTP_EXPORT_ALL="1",
                   REQUEST_METHOD=self.command, PATH_INFO=url.path, QUERY_STRING=url.query,
                   CONTENT_TYPE=self.headers.get("Content-Type", ""), CONTENT_LENGTH=str(length),
                   SERVER_PROTOCOL=self.request_version, REMOTE_USER="synthetic",
                   HTTP_GIT_PROTOCOL=self.headers.get("Git-Protocol", ""))
        result = run(["/usr/lib/git-core/git-http-backend"], env=env, input=self.rfile.read(length)).stdout
        header, separator, body = result.partition(b"\r\n\r\n")
        if not separator:
            header, separator, body = result.partition(b"\n\n")
        assert separator
        headers = [line.decode().split(":", 1) for line in header.splitlines()]
        status = next((int(value.strip().split()[0]) for name, value in headers if name.lower() == "status"), 200)
        self.send_response(status)
        for name, value in headers:
            if name.lower() != "status":
                self.send_header(name, value.strip())
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_GET = respond
    do_POST = respond


def server(handler, context=None):
    instance = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    instance.daemon_threads = True
    instance.observed = []
    instance.authenticated = 0
    instance.is_proxy = False
    if context:
        instance.socket = context.wrap_socket(instance.socket, server_side=True)
    thread = threading.Thread(target=instance.serve_forever, daemon=True)
    thread.start()
    return instance, thread


def stop(instance, thread):
    instance.shutdown()
    instance.server_close()
    thread.join(timeout=5)
    assert not thread.is_alive()


class Curl:
    def __init__(self, path):
        self.library = ctypes.CDLL(str(path))
        self.library.curl_global_init.argtypes = [ctypes.c_long]
        self.library.curl_global_init.restype = ctypes.c_int
        assert self.library.curl_global_init(3) == 0
        self.library.curl_easy_init.restype = ctypes.c_void_p
        self.library.curl_easy_cleanup.argtypes = [ctypes.c_void_p]
        self.library.curl_easy_perform.argtypes = [ctypes.c_void_p]
        self.library.curl_easy_perform.restype = ctypes.c_int
        self.library.curl_easy_setopt.restype = ctypes.c_int
        self.library.curl_version.restype = ctypes.c_char_p
        self.library.curl_slist_append.argtypes = [ctypes.c_void_p, ctypes.c_char_p]
        self.library.curl_slist_append.restype = ctypes.c_void_p
        self.library.curl_slist_free_all.argtypes = [ctypes.c_void_p]
        self.resolves = None
        self.version = self.library.curl_version().decode()
        self.handle = self.library.curl_easy_init()
        assert self.handle
        self.writer = ctypes.CFUNCTYPE(ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t, ctypes.c_size_t, ctypes.c_void_p)(lambda _ptr, size, count, _user: size * count)
        self.option(20011, self.writer)  # CURLOPT_WRITEFUNCTION
        self.option(155, 5000)  # CURLOPT_TIMEOUT_MS

    def option(self, number, value):
        if isinstance(value, int):
            argument = ctypes.c_long(value)
        elif isinstance(value, str):
            argument = ctypes.c_char_p(value.encode())
        elif value is None:
            argument = ctypes.c_void_p()
        else:
            argument = value
        assert self.library.curl_easy_setopt(ctypes.c_void_p(self.handle), ctypes.c_int(number), argument) == 0

    def perform(self, url):
        self.option(10002, url)  # CURLOPT_URL
        return self.library.curl_easy_perform(ctypes.c_void_p(self.handle))

    def resolve(self, entries):
        assert self.resolves is None
        for entry in entries:
            self.resolves = self.library.curl_slist_append(self.resolves, entry.encode())
            assert self.resolves
        self.option(10203, ctypes.c_void_p(self.resolves))  # CURLOPT_RESOLVE

    def close(self):
        self.library.curl_easy_cleanup(self.handle)
        self.library.curl_slist_free_all(self.resolves)


def credential_probes(paths, root):
    fixture, thread = server(HTTPFixture)
    failures = []
    try:
        for backend, path in paths.items():
            client = Curl(path)
            try:
                client.option(10004, f"http://127.0.0.1:{fixture.server_port}")  # CURLOPT_PROXY
                client.option(10177, "")  # CURLOPT_NOPROXY: use even for localhost
                client.option(111, 1)  # CURLOPT_PROXYAUTH: Basic
                client.option(10175, "synthetic-proxy")
                client.option(10176, "synthetic-password")
                assert client.perform("http://synthetic.invalid/control") == 0
                expected = "Basic " + base64.b64encode(b"synthetic-proxy:synthetic-password").decode()
                assert fixture.observed[-1]["proxyAuthorization"] == expected
                client.option(10006, None)  # CURLOPT_PROXYUSERPWD: explicitly clear
                assert client.perform("http://synthetic.invalid/cleared") == 0
                if fixture.observed[-1]["proxyAuthorization"] is not None:
                    failures.append(f"{backend}: cleared proxy credentials still sent")
                client.option(10004, None)
                client.option(10177, "*")
                netrc = root / (backend + ".netrc")
                netrc.write_text("machine 127.0.0.1 login synthetic-a password synthetic-password\n")
                netrc.chmod(0o600)
                client.option(10118, str(netrc))  # CURLOPT_NETRC_FILE
                client.option(51, 1)  # CURLOPT_NETRC: optional
                assert client.perform(f"http://synthetic-a@127.0.0.1:{fixture.server_port}/matching") == 0
                assert fixture.observed[-1]["authorization"] == "Basic " + base64.b64encode(b"synthetic-a:synthetic-password").decode()
                assert client.perform(f"http://synthetic-b@127.0.0.1:{fixture.server_port}/mismatch") == 0
                header = fixture.observed[-1]["authorization"]
                if header and b"synthetic-password" in base64.b64decode(header.split()[1]):
                    failures.append(f"{backend}: mismatched netrc user's password sent")
            finally:
                client.close()
    finally:
        stop(fixture, thread)
    if failures:
        raise AssertionError("; ".join(failures))
    print("PASS: both libcurl variants clear proxy credentials and isolate netrc users")


def digest_probes(paths):
    failures = []
    first, first_thread = server(DigestFixture)
    second, second_thread = server(HTTPFixture)
    proxy_keys = ["http_proxy", "https_proxy", "all_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"]
    original_env = {key: os.environ.get(key) for key in proxy_keys}
    try:
        for backend, path in paths.items():
            first.is_proxy = False
            first.authenticated = 0
            second.observed.clear()
            client = Curl(path)
            try:
                client.option(10177, "*")
                client.option(107, 2)  # CURLOPT_HTTPAUTH: Digest
                client.option(10173, "synthetic-user")
                client.option(10174, "synthetic-password")
                assert client.perform(f"http://localhost:{first.server_port}/digest") == 0
                assert first.authenticated == 1, f"{backend}: Digest origin control failed"
                assert client.perform(f"http://127.0.0.1:{second.server_port}/digest") == 0
                assert len(second.observed) == 1
                if second.observed[0]["authorization"] is not None:
                    failures.append(f"{backend}: origin Digest authorization crossed to another origin")
            finally:
                client.close()
            first.is_proxy = True
            first.authenticated = 0
            second.observed.clear()
            for key in proxy_keys:
                os.environ.pop(key, None)
            os.environ["http_proxy"] = f"http://127.0.0.1:{first.server_port}"
            client = Curl(path)
            try:
                client.option(10177, "")
                client.option(111, 2)  # CURLOPT_PROXYAUTH: Digest
                client.option(10175, "synthetic-proxy")
                client.option(10176, "synthetic-password")
                assert client.perform("http://synthetic.invalid/digest") == 0
                assert first.authenticated == 1, f"{backend}: Digest proxy control failed"
                os.environ["http_proxy"] = f"http://127.0.0.1:{second.server_port}"
                assert client.perform("http://synthetic.invalid/digest") == 0
                assert len(second.observed) == 1
                if second.observed[0]["proxyAuthorization"] is not None:
                    failures.append(f"{backend}: environment-selected proxy received another proxy's Digest authorization")
            finally:
                client.close()
    finally:
        for key, value in original_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        stop(first, first_thread)
        stop(second, second_thread)
    return failures


def cookie_and_cli_probes(paths, root):
    failures = []
    fixture, thread = server(HTTPFixture)
    try:
        port = fixture.server_port
        entries = [f"{host}:{port}:127.0.0.1" for host in ["example.co.uk.", "other.co.uk."]]
        for backend, path in paths.items():
            fixture.observed.clear()
            client = Curl(path)
            try:
                assert "libpsl/" in client.version, f"{backend}: PSL build prerequisite missing"
                client.option(10177, "*")
                client.option(10031, "")  # CURLOPT_COOKIEFILE: enable cookie engine
                client.resolve(entries)
                assert client.perform(f"http://example.co.uk.:{port}/set-cookie") == 0
                assert client.perform(f"http://example.co.uk.:{port}/same-host") == 0
                assert "legitimate=synthetic" in (fixture.observed[-1]["cookie"] or ""), f"{backend}: host cookie control failed"
                assert client.perform(f"http://other.co.uk.:{port}/other-host") == 0
                cookies = fixture.observed[-1]["cookie"] or ""
                assert "legitimate=synthetic" not in cookies
                if "super=synthetic" in cookies:
                    failures.append(f"{backend}: trailing-dot public-suffix cookie crossed hosts")
            finally:
                client.close()
        fixture.observed.clear()
        assert "PSL" in run(["curl", "--version"]).stdout.decode()
        command = ["curl", "--silent", "--show-error", "--noproxy", "*", "--cookie", ""]
        for entry in entries:
            command += ["--resolve", entry]
        run(command + [f"http://example.co.uk.:{port}/set-cookie", f"http://example.co.uk.:{port}/same-host", f"http://other.co.uk.:{port}/other-host"])
        assert len(fixture.observed) == 3
        assert "legitimate=synthetic" in (fixture.observed[1]["cookie"] or ""), "CLI host cookie control failed"
        cookies = fixture.observed[2]["cookie"] or ""
        assert "legitimate=synthetic" not in cookies
        if "super=synthetic" in cookies:
            failures.append("CLI: trailing-dot public-suffix cookie crossed hosts")
        netrc = root / "cli.netrc"
        netrc.write_text("machine 127.0.0.1 login synthetic-a password synthetic-password\n")
        netrc.chmod(0o600)
        command = ["curl", "--silent", "--show-error", "--noproxy", "*", "--netrc-optional", "--netrc-file", str(netrc)]
        run(command + [f"http://synthetic-a@127.0.0.1:{port}/matching"])
        assert fixture.observed[-1]["authorization"] == "Basic " + base64.b64encode(b"synthetic-a:synthetic-password").decode()
        run(command + [f"http://synthetic-b@127.0.0.1:{port}/mismatch"])
        header = fixture.observed[-1]["authorization"]
        if header and b"synthetic-password" in base64.b64decode(header.split()[1]):
            failures.append("CLI: mismatched netrc user's password sent")
    finally:
        stop(fixture, thread)
    return failures


def boundary_probes(paths, root):
    failures = digest_probes(paths) + cookie_and_cli_probes(paths, root)
    if failures:
        raise AssertionError("; ".join(failures))
    print("PASS: validated Digest controls and origin/environment-proxy isolation in both libraries")
    print("PASS: PSL trailing-dot cookie rejection in CLI/both libraries and CLI netrc user isolation")


def https_git(paths, root):
    cert, key = root / "certificate.pem", root / "tls-key.pem"
    run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=synthetic.invalid",
         "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1", "-keyout", str(key), "-out", str(cert)])
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(cert, key)
    fixture, thread = server(HTTPFixture, context)
    try:
        url = f"https://localhost:{fixture.server_port}/sdk"
        run(["curl", "--silent", "--show-error", "--noproxy", "*", "--cacert", str(cert), url])
        rejected = subprocess.run(["curl", "--silent", "--noproxy", "*", url], capture_output=True, timeout=10)
        assert rejected.returncode == 60
        for path in paths.values():
            client = Curl(path)
            try:
                client.option(10177, "*")
                assert client.perform(url) == 60
                client.option(10065, str(cert))  # CURLOPT_CAINFO
                assert client.perform(url) == 0
            finally:
                client.close()
    finally:
        stop(fixture, thread)
    project = root / "repos"
    project.mkdir()
    bare, seed, clone = project / "synthetic.git", root / "seed", root / "clone"
    run(["git", "init", "--bare", str(bare)])
    run(["git", "--git-dir=" + str(bare), "config", "http.receivepack", "true"])
    run(["git", "init", "-b", "main", str(seed)])
    (seed / "fixture.txt").write_text("synthetic-sdk\n")
    run(["git", "-C", str(seed), "add", "fixture.txt"])
    identity = ["-c", "user.name=Synthetic", "-c", "user.email=synthetic@invalid.example"]
    run(["git", "-C", str(seed)] + identity + ["commit", "-m", "synthetic fixture"])
    run(["git", "-C", str(seed), "push", str(bare), "main"])
    run(["git", "--git-dir=" + str(bare), "symbolic-ref", "HEAD", "refs/heads/main"])
    fixture, thread = server(GitFixture, context)
    fixture.project_root = project
    try:
        env = dict(os.environ, NO_PROXY="*", no_proxy="*")
        tls = ["-c", "http.sslCAInfo=" + str(cert)]
        run(["git"] + tls + ["clone", f"https://localhost:{fixture.server_port}/synthetic.git", str(clone)], env=env)
        assert (clone / "fixture.txt").read_text() == "synthetic-sdk\n"
        (clone / "fixture.txt").write_text("synthetic-sdk-updated\n")
        run(["git", "-C", str(clone), "add", "fixture.txt"])
        run(["git", "-C", str(clone)] + identity + ["commit", "-m", "synthetic update"])
        run(["git", "-C", str(clone)] + tls + ["push", "origin", "main"], env=env)
        assert run(["git", "--git-dir=" + str(bare), "rev-parse", "main"]).stdout == run(["git", "-C", str(clone), "rev-parse", "HEAD"]).stdout
    finally:
        stop(fixture, thread)
    print("PASS: CLI/both libcurl CA validation and Git HTTPS clone/commit/push")


def main():
    multiarch = {"aarch64": "aarch64-linux-gnu", "x86_64": "x86_64-linux-gnu"}[os.uname().machine]
    paths = {"openssl": Path(f"/usr/lib/{multiarch}/libcurl.so.4"), "gnutls": Path(f"/usr/lib/{multiarch}/libcurl-gnutls.so.4")}
    security_only = sys.argv[1:] == ["--security-only"]
    boundary_only = sys.argv[1:] == ["--boundary-only"]
    assert not sys.argv[1:] or security_only or boundary_only
    if not security_only and not boundary_only:
        record = json.loads(Path("/usr/local/share/claude-runtime/curl-build.json").read_text())
        assert record["version"] == "8.22.0" and record["releaseSignerFingerprint"] == "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
        assert hashlib.sha256(Path("/usr/bin/curl").read_bytes()).hexdigest() == record["cliSHA256"]
        for backend, variant in record["variants"].items():
            assert hashlib.sha256(Path(variant["libraryPath"]).read_bytes()).hexdigest() == variant["librarySHA256"]
            assert run(["dpkg-query", "-W", "-f=${Version}", {"openssl": "libcurl4t64", "gnutls": "libcurl3t64-gnutls"}[backend]]).stdout.decode() == record["packageVersion"]
            client = Curl(paths[backend])
            try:
                assert client.version.startswith("libcurl/8.22.0 ")
            finally:
                client.close()
        version = run(["curl", "--version"]).stdout.decode()
        assert "GnuTLS/" in version and "HTTP2" in version and "HTTP3" in version
        for feature in ["GSS-API", "Kerberos", "SPNEGO", "NTLM", "HTTPS-proxy"]:
            assert feature in version, feature
        for protocol in ["http", "https", "scp", "sftp", "smb", "smbs", "ldap", "ldaps"]:
            assert protocol in version.split("Protocols:", 1)[1].splitlines()[0].split(), protocol
    with tempfile.TemporaryDirectory(prefix="curl-sdk-") as directory:
        root = Path(directory)
        if not boundary_only:
            credential_probes(paths, root)
        if not security_only:
            boundary_probes(paths, root)
        if not security_only and not boundary_only:
            https_git(paths, root)


if __name__ == "__main__":
    main()
