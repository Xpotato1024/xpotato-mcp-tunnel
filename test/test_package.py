"""Package layout, integrity, permissions, and overwrite regression checks."""

import importlib.util
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location(
    "packaging", Path(__file__).resolve().parents[1] / "scripts/package-review.py")
packaging = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packaging)


class PackageTests(unittest.TestCase):
    def test_reproducible_release_archives(self):
        for platform in ("windows-amd64", "linux-amd64"):
            with self.subTest(platform=platform), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                binary = root / "binary"
                binary.write_bytes(b"fixed binary")
                inputs = dict(version="0.2.8", commit="b" * 40, platform=platform,
                              dirty=False, epoch=1792392030, release=True)
                first, content = packaging.assemble(binary, root / "first", **inputs,
                                                     build_info="/first/path: go1.27.2\n\tbuild\tGOARCH=amd64\n")
                os.utime(binary, (1000000000, 1000000000))
                second, _ = packaging.assemble(binary, root / "second", **inputs,
                                                build_info="C:\\other\\path: go1.27.2\n\tbuild\tGOARCH=amd64\n")
                self.assertEqual(packaging.sha256(first), packaging.sha256(second))
                self.assertNotIn("b" * 40, first.name)
                self.assertEqual(json.loads((content / "build-info.json").read_text())["sourceDateEpoch"], 1792392030)
                if platform.startswith("linux"):
                    with tarfile.open(first) as tar:
                        self.assertTrue(all(m.uid == 0 and m.gid == 0 and m.mtime == 1792392030
                                            for m in tar.getmembers()))

    def test_build_provenance(self):
        commit = "a" * 40
        info = (f"\tbuild\tGOOS=windows\n\tbuild\tGOARCH=amd64\n"
                f"\tbuild\tvcs.revision={commit}\n\tbuild\tvcs.modified=false\n")
        packaging.validate_build(info, commit, "windows-amd64")
        for altered, sha, platform in (
                (info, commit, "linux-amd64"),
                (info.replace("GOARCH=amd64", "GOARCH=arm64"), commit, "windows-amd64"),
                (info, "b" * 40, "windows-amd64"),
                (info.replace("modified=false", "modified=true"), commit, "windows-amd64"),
                ("", commit, "windows-amd64")):
            with self.subTest(info=altered, sha=sha, platform=platform), self.assertRaises(ValueError):
                packaging.validate_build(altered, sha, platform)

    def test_archive_layout_and_tamper_detection(self):
        for platform in ("windows-amd64", "linux-amd64"):
            with self.subTest(platform=platform), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                binary = root / "fixture"
                binary.write_bytes(b"test binary\x00\xff")
                output = root / "candidate"
                archive, content = packaging.assemble(
                    binary, output, "0.2.7", "a" * 40, platform, False, "test build\n")
                packaging.verify(content)
                metadata = json.loads((content / "build-info.json").read_text())
                self.assertFalse(metadata["runtimeIncluded"])
                self.assertTrue((content / "THIRD-PARTY-NOTICES.txt").is_file())
                self.assertEqual((content / "LICENSE").read_bytes(), (packaging.ROOT / "LICENSE").read_bytes())
                self.assertEqual((content / "COPYRIGHT").read_bytes(), (packaging.ROOT / "COPYRIGHT").read_bytes())
                self.assertTrue((content / "licensing.md").is_file())
                self.assertNotIn("../LICENSE",(content / "licensing.md").read_text(encoding="utf-8"))
                self.assertIn("[LICENSE](LICENSE)",(content / "licensing.md").read_text(encoding="utf-8"))
                self.assertIn("(licensing.md)",(content / "CHANGES.md").read_text(encoding="utf-8"))
                if platform.startswith("windows"):
                    self.assertTrue((content / "verify-windows.ps1").is_file())
                self.assertEqual(metadata["commit"], "a" * 40)
                self.assertIn("0.2.7-" + platform, archive.name)
                if platform.startswith("windows"):
                    with zipfile.ZipFile(archive) as z:
                        names = z.namelist()
                        self.assertIn("xpotato-tunnel.exe", names)
                        self.assertFalse(any(n.endswith((".zip", ".tar.gz")) for n in names))
                else:
                    with tarfile.open(archive) as tar:
                        self.assertEqual(tar.getmember("xpotato-tunnel").mode & 0o777, 0o755)
                self.assertEqual(packaging.sha256(archive),
                                 (output / (archive.name + ".sha256")).read_text().split()[0])
                (content / "README.md").write_text("corrupted")
                with self.assertRaisesRegex(ValueError, "SHA256 mismatch"):
                    packaging.verify(content)
                with self.assertRaises(FileExistsError):
                    packaging.assemble(binary, output, "0.2.7", "a" * 40, platform, False, "")

    def test_reject_unsafe_metadata(self):
        with tempfile.TemporaryDirectory() as temp:
            for version, commit, platform in (("../bad", "a" * 40, "windows-amd64"),
                                              ("0.2.7", "../bad", "windows-amd64"),
                                              ("0.2.7", "a" * 40, "windows-arm64")):
                with self.assertRaises(ValueError):
                    packaging.assemble(Path(temp), Path(temp) / "out", version,
                                       commit, platform, False, "")


if __name__ == "__main__":
    unittest.main()
