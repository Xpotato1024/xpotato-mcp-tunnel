"""Fixed release identity, server digest, failure/retry and rendered guide tests."""
import copy
import importlib.util
from pathlib import Path
import subprocess
import hashlib
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("release_guide", ROOT / "scripts/release-guide.py")
guide = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guide)


class ReleaseGuideTests(unittest.TestCase):
    def setUp(self):
        self.version = "0.2.10"
        self.commit = "a" * 40
        self.name = "xpotato-tunnel-0.2.10-windows-amd64.zip"
        self.release = dict(id=123, tag_name="v0.2.10", draft=True, prerelease=False,
                            html_url=guide.BASE + "/releases/tag/v0.2.10", assets=[dict(
                                id=456, name=self.name, state="uploaded", size=100,
                                digest="sha256:" + "b" * 64,
                                browser_download_url=guide.BASE + "/releases/download/v0.2.10/" + self.name)])

    def test_valid_and_repeatable(self):
        result = guide.validate(self.release, self.version, self.commit, self.commit)
        self.assertEqual(result["digest"], "b" * 64)
        body = guide.render(self.release, self.version, self.commit, self.commit)
        self.assertEqual(body, guide.render(self.release, self.version, self.commit, self.commit))
        self.assertIn(self.name, body)
        self.assertIn(self.commit, body)
        self.assertIn("b" * 64, body)
        self.assertIn("ReadAllBytes", body)
        self.assertNotIn(".zip.sha256", body)
        self.assertIn("Invoke-WebRequest", body)

    def test_reject_wrong_release_or_tag_commit(self):
        for field, value in (("id", 0), ("id", True), ("tag_name", "v0.2.9"),
                             ("html_url", "https://evil.test"), ("prerelease", True)):
            with self.subTest(field=field):
                release = copy.deepcopy(self.release)
                release[field] = value
                with self.assertRaises(ValueError):
                    guide.validate(release, self.version, self.commit, self.commit)
        for version, commit, actual in (("0.2.10';evil", self.commit, self.commit),
                                         (self.version, "bad", self.commit),
                                         (self.version, self.commit, "c" * 40)):
            with self.assertRaises(ValueError):
                guide.validate(self.release, version, commit, actual)

    def test_reject_missing_duplicate_wrong_or_incomplete_asset(self):
        for field, value in (("id", -1), ("id", True), ("state", "new"), ("size", 0),
                             ("size", True), ("digest", None), ("digest", "b" * 64),
                             ("digest", "sha256:" + "B" * 64),
                             ("browser_download_url", "https://evil.test/" + self.name),
                             ("name", "xpotato-tunnel-0.2.9-windows-amd64.zip")):
            with self.subTest(field=field, value=value):
                release = copy.deepcopy(self.release)
                release["assets"][0][field] = value
                with self.assertRaises(ValueError):
                    guide.validate(release, self.version, self.commit, self.commit)
        for assets in ([], self.release["assets"] * 2):
            release = dict(self.release, assets=assets)
            with self.assertRaises(ValueError):
                guide.validate(release, self.version, self.commit, self.commit)

    def test_api_failure_cancel_and_retry(self):
        for failure in (subprocess.CalledProcessError(1, ["gh"]), KeyboardInterrupt()):
            with patch.object(guide.subprocess, "check_output", side_effect=failure):
                with self.assertRaises(type(failure)):
                    guide.fetch(self.version)
        responses = [self.release, {"object": {"type": "tag", "sha": "d" * 40}},
                     {"object": {"type": "commit", "sha": self.commit}}]
        import json
        with patch.object(guide.subprocess, "check_output", side_effect=[json.dumps(x) for x in responses]) as call:
            release, commit = guide.fetch(self.version)
            self.assertEqual(commit, self.commit)
            self.assertEqual(release, self.release)
            self.assertEqual(call.call_count, 3)

    def test_local_archive_mismatch_does_not_write_output_then_retry(self):
        with tempfile.TemporaryDirectory() as temp:
            archive = Path(temp) / self.name
            output = Path(temp) / "guide.md"
            archive.write_bytes(b"actual ZIP fixture bytes")
            args = ["release-guide", "--version", self.version, "--commit", self.commit,
                    "--archive", str(archive), "--output", str(output)]
            with patch.object(sys, "argv", args), patch.object(guide, "fetch", return_value=(self.release, self.commit)):
                with self.assertRaisesRegex(ValueError, "differs"):
                    guide.main()
                self.assertFalse(output.exists())
                self.release["assets"][0].update(size=archive.stat().st_size,
                    digest="sha256:" + hashlib.sha256(archive.read_bytes()).hexdigest())
                guide.main()
                self.assertIn(self.commit, output.read_text(encoding="utf-8"))
                with self.assertRaises(FileExistsError):
                    guide.main()


if __name__ == "__main__":
    unittest.main()
