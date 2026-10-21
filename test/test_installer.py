"""Installer generation invariants; never compiles, installs or changes the OS."""
import importlib.util
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("installer", ROOT / "scripts/package-installer.py")
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)
fetch_spec = importlib.util.spec_from_file_location("fetch_installer", ROOT / "scripts/fetch-installer-tool.py")
fetcher = importlib.util.module_from_spec(fetch_spec)
fetch_spec.loader.exec_module(fetcher)


class InstallerTests(unittest.TestCase):
    def test_toolchain_digest_and_archive_traversal_fail_before_extraction(self):
        for unsafe in (False, True):
            with self.subTest(unsafe=unsafe), tempfile.TemporaryDirectory() as temp:
                content = io.BytesIO()
                with zipfile.ZipFile(content, "w") as archive:
                    archive.writestr("../escaped" if unsafe else "nsis-test/makensis.exe", b"fixture")
                data = content.getvalue()
                output = Path(temp) / "tool"
                pin = {"archiveSHA256": hashlib.sha256(data).hexdigest() if unsafe else "0" * 64}
                with self.assertRaises(ValueError):
                    fetcher.extract(data, output, pin)
                self.assertFalse(output.exists())
                self.assertFalse((Path(temp) / "escaped").exists())

    def test_pinned_toolchain_extracts_and_checks_compiler_and_license(self):
        with tempfile.TemporaryDirectory() as temp:
            content = io.BytesIO()
            with zipfile.ZipFile(content, "w") as archive:
                archive.writestr("nsis-test/makensis.exe", b"compiler fixture")
                archive.writestr("nsis-test/COPYING", b"license fixture")
            data = content.getvalue()
            pin = dict(version="test", archiveSHA256=hashlib.sha256(data).hexdigest(),
                       compilerSHA256=hashlib.sha256(b"compiler fixture").hexdigest(),
                       licenseSHA256=hashlib.sha256(b"license fixture").hexdigest())
            base = fetcher.extract(data, Path(temp) / "tool", pin)
            self.assertEqual((base / "COPYING").read_bytes(), b"license fixture")

    def test_explicit_file_lists_and_nonrecursive_removal(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            payload = root / "payload"
            (payload / "licenses").mkdir(parents=True)
            (payload / "xpotato-tunnel.exe").write_bytes(b"fixture")
            (payload / "licenses/LICENSE").write_text("fixture")
            installer.generate_includes(payload, root)
            install = (root / "install-files.nsh").read_text()
            remove = (root / "uninstall-files.nsh").read_text()
            self.assertEqual(install.count("  File "), 2)
            self.assertIn('Delete "$INSTDIR\\licenses\\LICENSE"', remove)
            self.assertNotIn("/r", remove.lower())
            self.assertNotIn("*", remove)
            self.assertNotIn('Delete "$INSTDIR\\xpotato-tunnel.exe"', remove)

    def test_paths_cannot_inject_nsis_source(self):
        for path in ('$INSTDIR', 'a"b', "a\nb", "a\rb", "a\0b"):
            with self.subTest(path=path), self.assertRaises(ValueError):
                installer.nsis_literal(path)

    def test_no_elevation_process_kill_recursive_purge_or_autostart(self):
        script = (ROOT / "installer/windows.nsi").read_text()
        self.assertIn("RequestExecutionLevel user", script)
        self.assertIn("SetCompressor /SOLID zlib", script)
        for forbidden in ("RMDir /r", "Delete /REBOOTOK", "taskkill", "HKLM", "RunOnce", "sc.exe", "ExecutionPolicy"):
            self.assertNotIn(forbidden, script)


if __name__ == "__main__":
    unittest.main()
