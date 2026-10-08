"""Build-time archive boundaries: invalid inputs must leave the installed bundle intact."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('npm_overrides',
        Path(__file__).resolve().parents[1] / 'images/runtime/install-npm-overrides.py')
overrides = importlib.util.module_from_spec(spec)
spec.loader.exec_module(overrides)


class ArchiveTests(unittest.TestCase):
    def archive(self, extra=None, identity='synthetic', license=True):
        data = io.BytesIO()
        files = {'package/package.json': json.dumps({'name': identity, 'version': '1.0.1'}).encode(),
                 'package/index.js': b'module.exports = 17'}
        if license:
            files['package/LICENSE'] = b'Original synthetic license bytes\n'
        with tarfile.open(fileobj=data, mode='w:gz') as archive:
            for name, raw in files.items():
                entry = tarfile.TarInfo(name); entry.size = len(raw)
                archive.addfile(entry, io.BytesIO(raw))
            if extra:
                archive.addfile(extra, io.BytesIO(b'x') if extra.isfile() else None)
        return data.getvalue()

    def exercise(self, raw, passes=False, checksum=None):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch) / 'npm'
            installed = root / 'node_modules/synthetic'; installed.mkdir(parents=True)
            (installed / 'old').write_text('must survive rejected input')
            pin = {'name': 'synthetic', 'version': '1.0.1', 'dependencies': {},
                   'sha512': checksum or hashlib.sha512(raw).hexdigest()}
            with patch.object(overrides, 'ROOT', root), \
                 patch.object(overrides.urllib.request, 'urlopen', return_value=io.BytesIO(raw)), \
                 patch.object(overrides, 'compatible'):
                if passes:
                    overrides.install(pin)
                    self.assertFalse((installed / 'old').exists())
                    self.assertEqual((installed / 'LICENSE').read_bytes(), b'Original synthetic license bytes\n')
                    self.assertEqual((installed / 'index.js').read_bytes(), b'module.exports = 17')
                else:
                    with self.assertRaises((RuntimeError, KeyError, FileNotFoundError)):
                        overrides.install(pin)
                    self.assertEqual((installed / 'old').read_text(), 'must survive rejected input')

    def test_original_bytes_and_replacement(self):
        self.exercise(self.archive(), passes=True)

    def test_checksum_before_replacement(self):
        self.exercise(self.archive(), checksum='0' * 128)

    def test_identity_and_license_before_replacement(self):
        self.exercise(self.archive(identity='different'))
        self.exercise(self.archive(license=False))

    def test_paths_links_and_duplicates_before_replacement(self):
        for name in ['../escape', '/escape', 'package/../escape', 'package/./index.js', 'package/index.js']:
            with self.subTest(name=name):
                entry = tarfile.TarInfo(name); entry.size = 1
                self.exercise(self.archive(extra=entry))
        for kind in [tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.FIFOTYPE]:
            with self.subTest(kind=kind):
                entry = tarfile.TarInfo('package/link'); entry.type = kind; entry.linkname = '/escape'
                self.exercise(self.archive(extra=entry))


if __name__ == '__main__':
    unittest.main()
