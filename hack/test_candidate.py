"""Artifact-boundary tests independent of the producer and signing infrastructure."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("candidate_verifier", Path(__file__).with_name("verify-candidate.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


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


if __name__ == "__main__":
    unittest.main()
