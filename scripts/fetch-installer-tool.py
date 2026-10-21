"""Fetch the pinned portable NSIS toolchain into a new development directory."""
import argparse
import hashlib
import html
import io
import json
from pathlib import Path, PurePosixPath
import re
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def fetch(url):
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read(10 * 1024 * 1024 + 1)
    if len(data) > 10 * 1024 * 1024:
        raise ValueError("toolchain response exceeds limit")
    return data


def extract(data, output, pin):
    if hashlib.sha256(data).hexdigest() != pin["archiveSHA256"]:
        raise ValueError("toolchain archive SHA256 mismatch; nothing executed")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        for entry in archive.infolist():
            name = entry.filename
            if ("\\" in name or ":" in name or "\x00" in name or name.startswith("/") or
                    ".." in PurePosixPath(name).parts or
                    ((entry.external_attr >> 16) & 0o170000) == 0o120000):
                raise ValueError("unsafe toolchain archive path")
        output.mkdir(parents=True, exist_ok=False)
        archive.extractall(output)
    base = output / ("nsis-" + pin["version"])
    for name, expected in (("makensis.exe", pin["compilerSHA256"]), ("COPYING", pin["licenseSHA256"])):
        if hashlib.sha256((base / name).read_bytes()).hexdigest() != expected:
            raise ValueError("extracted toolchain pin mismatch")
    return base


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    pin = json.loads((ROOT / "installer/nsis-toolchain.json").read_text(encoding="utf-8"))
    data = fetch(pin["url"])
    if not zipfile.is_zipfile(io.BytesIO(data)):
        # SourceForge sometimes returns a timed-download page with a signed
        # public download URL. Follow that vendor link once, never run HTML.
        match = re.search(r'<meta http-equiv="refresh" content="\d+;\s*url=([^"]+)"', data.decode("utf-8"))
        url = html.unescape(match[1]) if match else ""
        if not url.startswith(pin["url"] + "?"):
            raise ValueError("vendor did not provide the pinned ZIP")
        data = fetch(url)
    print(extract(data, args.output.resolve(), pin))


if __name__ == "__main__":
    main()
