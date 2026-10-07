#!/usr/bin/env python3
"""Apply verified upstream patch releases to npm's bundled dependencies at build time."""
import hashlib
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request


ROOT = Path('/usr/local/lib/node_modules/npm')
PINS = [
    {
        'name': 'brace-expansion', 'version': '5.0.11',
        'sha512': '6b08a08e18ba70b4e1f746ddc3af9027d0ad9899b26211088bbd2209b73c468ce7d38170f457902018effc4db61451ae086c756cb27251f07ae31feccd449752',
        'dependencies': {'balanced-match': '^4.0.2'},
        'advisories': ['GHSA-6j4f-fj2g-mc7p', 'GHSA-qhr7-859c-m2p7'],
    },
    {
        'name': 'undici', 'version': '6.28.1',
        'sha512': 'cd6a5d4d50f9e07e3c088c9b2f4ad6437ba4a5bf5ddb7c0cede1f946d7dd99e3fbd1c587363b5fa3b3f8bd95fee42a0370ee772783eee6e5e9ee535f8dce8344',
        'dependencies': {}, 'advisories': ['GHSA-rfgv-xxqx-mfg5'],
    },
]


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def compatible(pin, package):
    # Reject future npm changes whose actual parent ranges or installed prerequisites differ.
    checks = []
    for path in [ROOT / 'package.json', *(ROOT / 'node_modules').rglob('package.json')]:
        parent = json.loads(path.read_text())
        for kind in ('dependencies', 'optionalDependencies'):
            constraint = parent.get(kind, {}).get(pin['name'])
            if constraint:
                checks.append([pin['version'], constraint])
    require(checks, 'no parent dependency range found for ' + pin['name'])
    for name, constraint in package.get('dependencies', {}).items():
        dependency = json.loads((ROOT / 'node_modules' / name / 'package.json').read_text())
        checks.append([dependency['version'], constraint])
    subprocess.run(['node', '-e', '''
const fs = require('fs');
const semver = require('/usr/local/lib/node_modules/npm/node_modules/semver');
for (const [version, range] of JSON.parse(fs.readFileSync(0, 'utf8'))) {
  if (!semver.satisfies(version, range)) throw new Error('incompatible npm dependency range');
}
'''], input=json.dumps(checks), text=True, check=True)


def install(pin):
    url = f"https://registry.npmjs.org/{pin['name']}/-/{pin['name']}-{pin['version']}.tgz"
    with tempfile.TemporaryDirectory(prefix='claude-npm-patch-') as scratch:
        staging = Path(scratch)
        with urllib.request.urlopen(url, timeout=60) as response:
            raw = response.read(8 * 1024 * 1024 + 1)
        require(len(raw) <= 8 * 1024 * 1024, 'npm patch archive too large')
        require(hashlib.sha512(raw).hexdigest() == pin['sha512'], 'npm patch checksum mismatch')
        archive = staging / 'package.tgz'
        archive.write_bytes(raw)
        extracted = staging / 'extracted'
        extracted.mkdir()
        with tarfile.open(archive) as source:
            members = source.getmembers()
            require(len(members) <= 1024 and sum(m.size for m in members) <= 32 * 1024 * 1024,
                    'npm patch contents too large')
            seen = set()
            for member in members:
                path = PurePosixPath(member.name)
                require(not path.is_absolute() and path.parts and path.parts[0] == 'package'
                        and '..' not in path.parts and path.as_posix() == member.name
                        and member.name not in seen,
                        'unsafe or duplicate npm patch path')
                require(member.isfile() or member.isdir(), 'npm patch links/special files forbidden')
                seen.add(member.name)
                target = extracted.joinpath(*path.parts)
                if member.isdir():
                    target.mkdir(parents=True, exist_ok=True)
                else:
                    target.parent.mkdir(parents=True, exist_ok=True)
                    with source.extractfile(member) as data:
                        target.write_bytes(data.read())
                    target.chmod(0o644)
        package = json.loads((extracted / 'package/package.json').read_text())
        require(package['name'] == pin['name'] and package['version'] == pin['version']
                and package.get('dependencies', {}) == pin['dependencies'], 'npm patch identity changed')
        require((extracted / 'package/LICENSE').is_file(), 'npm patch license missing')
        compatible(pin, package)
        destination = ROOT / 'node_modules' / pin['name']
        require(destination.is_dir() and not destination.is_symlink(), 'npm bundle destination invalid')
        shutil.rmtree(destination)
        shutil.move(str(extracted / 'package'), destination)
    return {**pin, 'archiveURL': url}


if __name__ == '__main__':
    require(json.loads((ROOT / 'package.json').read_text())['version'] == '11.21.0',
            'security overrides require the reviewed npm version')
    applied = [install(pin) for pin in PINS]
    record = Path('/usr/local/share/claude-runtime/npm-security-overrides.json')
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps({'npmVersion': '11.21.0', 'overrides': applied}, indent=2) + '\n')
