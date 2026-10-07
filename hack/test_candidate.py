"""Artifact-boundary tests independent of the producer and signing infrastructure."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import tarfile
import unittest

spec = importlib.util.spec_from_file_location("candidate_verifier", Path(__file__).with_name("verify-candidate.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)

notice_spec = importlib.util.spec_from_file_location("notice_collector", Path(__file__).with_name("collect-notices.py"))
notices = importlib.util.module_from_spec(notice_spec)
notice_spec.loader.exec_module(notices)


class InventoryTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        (self.root / "binary").write_bytes(b"synthetic-binary")
        self.manifest = {"schemaVersion": 1, "kind": "review-candidate", "sourceCommit": "a" * 40,
                         "files": {"binary": {"bytes": 16, "sha256": hashlib.sha256(b"synthetic-binary").hexdigest()}}}

    def test_valid_inventory(self):
        verifier.verify_inventory(self.root, self.manifest)

    def test_same_size_tamper(self):
        (self.root / "binary").write_bytes(b"synthetic-BINARY")
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)

    def test_missing_or_added_file(self):
        (self.root / "unlisted").write_text("unexpected")
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)
        (self.root / "unlisted").unlink()
        (self.root / "binary").unlink()
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)

    def test_symlink(self):
        (self.root / "binary").unlink()
        (self.root / "binary").symlink_to(Path(__file__))
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)

    def test_paths_cannot_escape_directory(self):
        self.manifest["files"]["../outside"] = self.manifest["files"].pop("binary")
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)

    def test_unknown_schema_and_invalid_digest(self):
        self.manifest["schemaVersion"] = 2
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)
        self.manifest["schemaVersion"] = 1
        self.manifest["files"]["binary"]["sha256"] = "not-a-digest"
        with self.assertRaises(ValueError):
            verifier.verify_inventory(self.root, self.manifest)


class NoticeTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        (self.root / "LICENSE-MIT").write_bytes(b"Synthetic license\r\n")
        (self.root / "nested").mkdir()
        (self.root / "nested/PATENTS").write_bytes(b"Synthetic patent grant\n")
        (self.root / "nested/THIRD_PARTY_NOTICES.txt").write_bytes(b"Synthetic notice\n")
        (self.root / "unrelated.txt").write_bytes(b"Not a notice")

    def test_original_bytes_nested_notices_and_normalized_archive(self):
        files = notices.notice_files(self.root)
        self.assertEqual(set(files), {"LICENSE-MIT", "nested/PATENTS", "nested/THIRD_PARTY_NOTICES.txt"})
        for name in ["first", "second"]:
            destination = self.root / name
            destination.mkdir()
            index = notices.write_inventory(destination, [({"kind": "module", "path": "synthetic.example/module"}, files)], {})
            self.assertFalse(index["licenseReviewApproved"])
            with tarfile.open(destination / "third-party-notices.tar.gz") as archive:
                for member in archive.getmembers():
                    record = next(record for record in index["sources"][0]["files"] if record["archivePath"] == member.name)
                    self.assertTrue(member.isfile())
                    self.assertEqual(member.uid, 0)
                    self.assertEqual(member.mtime, 0)
                    content = archive.extractfile(member).read()
                    self.assertEqual(content, files[record["sourcePath"]])
                    self.assertEqual(hashlib.sha256(content).hexdigest(), record["sha256"])
        self.assertEqual((self.root / "first/third-party-notices.tar.gz").read_bytes(),
                         (self.root / "second/third-party-notices.tar.gz").read_bytes())

    def test_missing_license_and_symlink_require_review(self):
        (self.root / "LICENSE-MIT").unlink()
        with self.assertRaises(ValueError):
            notices.notice_files(self.root)
        (self.root / "LICENSE").symlink_to(Path(__file__))
        with self.assertRaises(ValueError):
            notices.notice_files(self.root)

    def test_module_identity_conflicts_and_replacements_rejected(self):
        info = "binary: go1.27.1\n\tdep\texample.com/module\tv1.0.0\th1:synthetic\n"
        version, modules = notices.compiled_modules({"amd64": info, "arm64": info})
        self.assertEqual(version, "go1.27.1")
        self.assertEqual(modules[0]["binaries"], ["amd64", "arm64"])
        for changed in [info.replace("h1:synthetic", "h1:other"), info + "\t=>\t../local\t(devel)\n",
                        info.replace("go1.27.1", "go1.26.5"), info.replace("h1:synthetic", "")]:
            with self.assertRaises(ValueError):
                notices.compiled_modules({"amd64": info, "arm64": changed})

    def test_path_escape_and_oversized_notice_rejected(self):
        destination = self.root / "output"
        destination.mkdir()
        with self.assertRaises(ValueError):
            notices.write_inventory(destination, [({}, {"../escape": b"synthetic"})], {})
        (self.root / "LICENSE-MIT").write_bytes(b"x" * (notices.MAX_FILE_BYTES + 1))
        with self.assertRaises(ValueError):
            notices.notice_files(self.root)


if __name__ == "__main__":
    unittest.main()
