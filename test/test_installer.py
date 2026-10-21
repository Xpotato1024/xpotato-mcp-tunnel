"""Installer generation invariants; never compiles, installs or changes the OS."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("installer", ROOT / "scripts/package-installer.py")
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallerTests(unittest.TestCase):
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
