#!/usr/bin/env python3
# Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
"""Build verified upstream curl and both ABI-compatible Debian libcurl flavours."""

import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request

VERSION = "8.22.0"
PACKAGE_VERSION = VERSION + "-operator1"
FINGERPRINT = "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
ASSETS = {
    "source": ("https://curl.se/download/curl-8.22.0.tar.xz", "f7ef3ae8a22e521f289803fe93543eb64c329b58aa73a9e224dfd915a2a5f4f7", 8 * 1024 * 1024),
    "signature": ("https://curl.se/download/curl-8.22.0.tar.xz.asc", "ff9acdd6e48690d037b85337e42320b87aead3414ba83b75e77ec08d2eea908e", 16 * 1024),
    "signer": (
        "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x27edeaf22f3abceb50db9a125cc908fdb71e12c2",
        "a02c65495a46ce084162aa2143ebc588a136798ece5089ea85dce1d0691c87ef", 256 * 1024),
}


def fetch(name, directory):
    url, digest, limit = ASSETS[name]
    with urllib.request.urlopen(url, timeout=60) as response:
        if response.geturl().split(":", 1)[0] != "https":
            raise ValueError("non-HTTPS asset redirect")
        data = response.read(limit + 1)
    if len(data) > limit or hashlib.sha256(data).hexdigest() != digest:
        raise ValueError(f"invalid pinned {name} asset")
    path = directory / name
    path.write_bytes(data)
    return path


def package(name, root, architecture, depends, output):
    control = root / "DEBIAN"
    control.mkdir()
    (control / "control").write_text(
        f"Package: {name}\nVersion: {PACKAGE_VERSION}\nSource: curl\nArchitecture: {architecture}\n"
        "Maintainer: Claude Code Cloud Operator maintainers <noreply@users.noreply.github.com>\n"
        f"Depends: {depends}\nSection: libs\nPriority: optional\n"
        "Description: Privately built upstream curl candidate, not a Debian release\n"
    )
    files = sorted(p for p in root.rglob("*") if p.is_file() and not p.is_symlink() and control not in p.parents)
    (control / "md5sums").write_text("".join(f"{hashlib.md5(p.read_bytes()).hexdigest()}  {p.relative_to(root)}\n" for p in files))
    subprocess.run(["dpkg-deb", "--root-owner-group", "--build", str(root), str(output / f"{name}.deb")], check=True)


def main():
    output = Path("/opt/curl-packages")
    output.mkdir()
    architecture = subprocess.check_output(["dpkg", "--print-architecture"], text=True).strip()
    multiarch = subprocess.check_output(["gcc", "-print-multiarch"], text=True).strip()
    dependencies = {name: subprocess.check_output(["dpkg-query", "-W", "-f=${Depends}", name], text=True)
                    for name in ("curl", "libcurl4t64", "libcurl3t64-gnutls")}
    with tempfile.TemporaryDirectory(prefix="curl-build-") as directory:
        root = Path(directory)
        assets = {name: fetch(name, root) for name in ASSETS}
        home = root / "trust"
        home.mkdir(mode=0o700)
        ring = home / "trusted.gpg"
        subprocess.run(["gpg", "--no-options", "--no-autostart", "--homedir", str(home), "--batch", "--output", str(ring), "--dearmor", str(assets["signer"])], check=True)
        verified = subprocess.run(["gpgv", "--homedir", str(home), "--keyring", str(ring), "--status-fd", "1", str(assets["signature"]), str(assets["source"])], capture_output=True, text=True, check=True)
        if not any(line.startswith(f"[GNUPG:] VALIDSIG {FINGERPRINT} ") for line in verified.stdout.splitlines()):
            raise ValueError("unexpected release signer")
        print(f"Verified curl {VERSION} release signer {FINGERPRINT}", flush=True)
        with tarfile.open(assets["source"]) as archive:
            archive.extractall(root, filter="data")
        source = root / f"curl-{VERSION}"
        record = {"version": VERSION, "packageVersion": PACKAGE_VERSION, "releaseSignerFingerprint": FINGERPRINT,
                  "assets": {name: {"url": item[0], "sha256": item[1]} for name, item in ASSETS.items()},
                  "sourceModified": False, "disputedAmbientUserAdvisoryResolved": False,
                  "cliTLSBackend": "gnutls", "variants": {}}
        for backend, name in (("openssl", "libcurl4t64"), ("gnutls", "libcurl3t64-gnutls")):
            build = root / backend
            build.mkdir()
            configure = [str(source / "configure"), "--prefix=/usr", f"--libdir=/usr/lib/{multiarch}",
                         "--enable-shared", "--disable-static", "--enable-versioned-symbols", "--enable-symbol-hiding", "--enable-ntlm", "--enable-smb",
                         "--with-ca-bundle=/etc/ssl/certs/ca-certificates.crt", "--with-ca-path=/etc/ssl/certs",
                         "--with-gssapi", "--with-libssh2", "--with-nghttp2", "--with-libidn2", "--with-brotli", "--with-zstd", "--with-zlib", "--with-libpsl",
                         f"--with-{backend}"]
            if backend == "gnutls":
                configure += ["--with-ngtcp2", "--with-nghttp3"]
            subprocess.run(configure, cwd=build, check=True)
            if backend == "gnutls":
                # Debian's SONAME is .so.4 but its historical symbol namespace is _3.
                # Change the generated linker script, without modifying upstream C source.
                version_script = build / "lib/libcurl.vers"
                symbols = version_script.read_text()
                if symbols.count("CURL_GNUTLS_4") != 1:
                    raise ValueError("unexpected generated GnuTLS symbol namespace")
                version_script.write_text(symbols.replace("CURL_GNUTLS_4", "CURL_GNUTLS_3"))
            subprocess.run(["make", "-j2"], cwd=build, check=True)
            installed = root / (backend + "-installed")
            subprocess.run(["make", "install", "DESTDIR=" + str(installed)], cwd=build, check=True)
            package_root = root / name
            library_dir = package_root / f"usr/lib/{multiarch}"
            library_dir.mkdir(parents=True)
            library = next((installed / f"usr/lib/{multiarch}").glob("libcurl.so.4.*"))
            target_name = library.name.replace("libcurl", "libcurl-gnutls") if backend == "gnutls" else library.name
            target = library_dir / target_name
            shutil.copy2(library, target)
            soname = "libcurl-gnutls.so.4" if backend == "gnutls" else "libcurl.so.4"
            if backend == "gnutls":
                # Debian Git binds this separate SONAME and CURL_GNUTLS_3 symbol namespace.
                subprocess.run(["patchelf", "--set-soname", soname, str(target)], check=True)
            (library_dir / soname).symlink_to(target_name)
            notices = package_root / f"usr/share/doc/{name}"
            notices.mkdir(parents=True)
            shutil.copy2(source / "COPYING", notices / "copyright")
            shutil.copy2(source / "COPYING", notices / "COPYING")
            original_symbols = subprocess.check_output(["readelf", "--dyn-syms", "--wide", f"/usr/lib/{multiarch}/{soname}"], text=True)
            symbols = subprocess.check_output(["readelf", "--dyn-syms", "--wide", str(target)], text=True)
            # Every exported versioned curl symbol required by the previous library must remain.
            old = {line.split()[-1] for line in original_symbols.splitlines() if "@@CURL_" in line}
            new = {line.split()[-1] for line in symbols.splitlines() if "@@CURL_" in line}
            if not old or not old <= new:
                raise ValueError(f"missing {backend} ABI symbols: {sorted(old - new)}")
            package(name, package_root, architecture, dependencies[name], output)
            record["variants"][backend] = {"libraryPath": f"/usr/lib/{multiarch}/{target_name}", "librarySHA256": hashlib.sha256(target.read_bytes()).hexdigest(),
                                           "soname": soname, "previousExportedSymbolsPreserved": len(old), "configure": [Path(configure[0]).name] + configure[1:],
                                           "symbolNamespace": "CURL_GNUTLS_3" if backend == "gnutls" else "CURL_OPENSSL_4",
                                           "generatedSymbolNamespaceAdjusted": backend == "gnutls",
                                           "configSHA256": hashlib.sha256((build / "lib/curl_config.h").read_bytes()).hexdigest()}
            if backend == "gnutls":
                cli_root = root / "curl-cli"
                (cli_root / "usr/bin").mkdir(parents=True)
                shutil.copy2(installed / "usr/bin/curl", cli_root / "usr/bin/curl")
                # Current upstream removed OpenSSL-QUIC; the GnuTLS CLI retains HTTP/3.
                subprocess.run(["patchelf", "--replace-needed", "libcurl.so.4", "libcurl-gnutls.so.4", str(cli_root / "usr/bin/curl")], check=True)
                (cli_root / "usr/share/doc/curl").mkdir(parents=True)
                shutil.copy2(source / "COPYING", cli_root / "usr/share/doc/curl/copyright")
                (cli_root / "usr/share/man/man1").mkdir(parents=True)
                shutil.copy2(installed / "usr/share/man/man1/curl.1", cli_root / "usr/share/man/man1/curl.1")
                package("curl", cli_root, architecture, f"libcurl3t64-gnutls (= {PACKAGE_VERSION}), libc6 (>= 2.41)", output)
                record["cliSHA256"] = hashlib.sha256((cli_root / "usr/bin/curl").read_bytes()).hexdigest()
        (output / "curl-build.json").write_text(json.dumps(record, indent=2) + "\n")


if __name__ == "__main__":
    main()
