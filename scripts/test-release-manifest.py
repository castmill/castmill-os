#!/usr/bin/env python3
"""Tests for release-manifest.py (run: python3 scripts/test-release-manifest.py)."""
import datetime
import hashlib
import importlib.util
import os
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "release_manifest", os.path.join(os.path.dirname(__file__), "release-manifest.py")
)
rm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rm)

NOW = datetime.datetime(2026, 12, 1, 10, 0, tzinfo=datetime.timezone.utc)


class ManifestTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.dist = self.dir.name
        for name, data in {
            "castmill-os-bundle-genericx86-64.raucb": b"x86",
            "castmill-os-bundle-raspberrypi5.raucb": b"pi5!",
            "castmill-os-image-raspberrypi5.rootfs.wic.zst": b"image",
            "SHA256SUMS": b"",
        }.items():
            with open(os.path.join(self.dist, name), "wb") as f:
                f.write(data)

    def tearDown(self):
        self.dir.cleanup()

    def test_release(self):
        m = rm.manifest(self.dist, "v0.3.0", "castmill/castmill-os", NOW)
        self.assertEqual(m["version"], "0.3.0")
        self.assertFalse(m["prerelease"])
        self.assertEqual(m["published_at"], "2026-12-01T10:00:00Z")
        self.assertEqual(sorted(m["artifacts"]), ["castmill-os-genericx86-64", "castmill-os-raspberrypi5"])
        pi = m["artifacts"]["castmill-os-raspberrypi5"]
        self.assertEqual(
            pi["url"],
            "https://github.com/castmill/castmill-os/releases/download/v0.3.0/castmill-os-bundle-raspberrypi5.raucb",
        )
        self.assertEqual(pi["sha256"], hashlib.sha256(b"pi5!").hexdigest())
        self.assertEqual(pi["size"], 4)

    def test_prerelease(self):
        self.assertTrue(rm.manifest(self.dist, "v0.3.0-rc.1", "o/r", NOW)["prerelease"])

    def test_rejects(self):
        with self.assertRaises(SystemExit):
            rm.manifest(self.dist, "main", "o/r", NOW)
        empty = tempfile.TemporaryDirectory()
        with self.assertRaises(SystemExit):
            rm.manifest(empty.name, "v0.3.0", "o/r", NOW)
        empty.cleanup()


if __name__ == "__main__":
    unittest.main()
