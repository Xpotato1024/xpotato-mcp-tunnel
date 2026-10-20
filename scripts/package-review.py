"""Assemble native review/release candidates locally. Never uploads or tags."""

import argparse
import datetime
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DOCS = ("quickstart.md", "setup-once-core.md", "operations.md", "setup-env.md",
        "runtime-ux.md", "performance-review.md", "distribution.md", "update-transactions.md")


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_build(build_info, commit, platform):
    settings = dict(re.findall(r"^\s*build\s+(\S+)=(\S+)\s*$", build_info, re.MULTILINE))
    target_os, target_arch = platform.split("-")
    if settings.get("GOOS") != target_os or settings.get("GOARCH") != target_arch:
        raise ValueError("binary OS/architecture differs from package platform")
    if settings.get("vcs.revision") != commit or settings.get("vcs.modified") != "false":
        raise ValueError("binary must be built from the clean current commit")


def verify(content):
    for line in (content / "SHA256SUMS").read_text(encoding="utf-8").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([\w./-]+)", line)
        if not match or ".." in Path(match[2]).parts or match[2].startswith("/"):
            raise ValueError("invalid checksum entry")
        if sha256(content / match[2]) != match[1]:
            raise ValueError(f"SHA256 mismatch: {match[2]}")


def assemble(binary, output, version, commit, platform, dirty, build_info,
             epoch=315532800, release=False):
    if platform not in ("windows-amd64", "linux-amd64"):
        raise ValueError("unsupported platform")
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("invalid version")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("invalid commit")
    # Exclusive output prevents mixing candidates or overwriting previous evidence.
    output.mkdir(parents=True, exist_ok=False)
    content = output / "content"
    content.mkdir()
    name = "xpotato-tunnel.exe" if platform.startswith("windows") else "xpotato-tunnel"
    shutil.copy2(binary, content / name)
    if platform.startswith("linux"):
        (content / name).chmod(0o755)
    for doc in DOCS:
        shutil.copy2(ROOT / "docs" / doc, content / doc)
    shutil.copy2(ROOT / "docs/quickstart.md", content / "README.md")
    shutil.copy2(ROOT / "THIRD-PARTY-NOTICES.txt", content / "THIRD-PARTY-NOTICES.txt")
    if platform.startswith("windows"):
        shutil.copy2(ROOT / "scripts/verify-windows.ps1", content / "verify-windows.ps1")
    shutil.copytree(ROOT / "docs/performance-results", content / "performance-results")
    shutil.copy2(ROOT / "setup.env.example", content / "setup.env.example")
    # The executable path printed by `go version -m` depends on the build machine.
    lines = build_info.splitlines()
    if lines and ": " in lines[0]:
        lines[0] = f"{name}: {lines[0].rsplit(': ', 1)[1]}"
    (content / "build-info.txt").write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    metadata = dict(version=version, commit=commit, platform=platform, dirty=dirty,
                    runtimeIncluded=False, binary=name, sourceDateEpoch=epoch)
    (content / "build-info.json").write_text(
        json.dumps(metadata, indent=2) + "\n", encoding="utf-8", newline="\n")
    files = sorted(p for p in content.rglob("*") if p.is_file())
    (content / "SHA256SUMS").write_text("".join(
        f"{sha256(p)}  {p.relative_to(content).as_posix()}\n" for p in files),
        encoding="utf-8", newline="\n")
    stem = f"xpotato-tunnel-{version}-{platform}" + ("" if release else f"-{commit}")
    archive = output / (stem + (".zip" if platform.startswith("windows") else ".tar.gz"))
    if platform.startswith("windows"):
        date = datetime.datetime.fromtimestamp(max(epoch, 315532800), datetime.timezone.utc)
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for p in sorted(content.rglob("*")):
                if p.is_file():
                    info = zipfile.ZipInfo(p.relative_to(content).as_posix(), date.timetuple()[:6])
                    info.create_system = 3
                    info.external_attr = 0o100644 << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    z.writestr(info, p.read_bytes(), compresslevel=9)
    else:
        with archive.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=epoch) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as tar:
                for p in sorted(content.rglob("*")):
                    if p.is_file():
                        info = tarfile.TarInfo(p.relative_to(content).as_posix())
                        info.mode = 0o755 if info.name == name else 0o644
                        info.mtime = epoch
                        info.size = p.stat().st_size
                        with p.open("rb") as source:
                            tar.addfile(info, source)
    (output / (archive.name + ".sha256")).write_text(
        f"{sha256(archive)}  {archive.name}\n", encoding="utf-8", newline="\n")
    # Verify the actual distributable after extraction, including executable mode.
    with tempfile.TemporaryDirectory(prefix="xpotato-package-") as temp:
        extracted = Path(temp)
        shutil.unpack_archive(str(archive), temp)
        verify(extracted)
        if platform.startswith("linux") and not os.access(extracted / name, os.X_OK):
            raise ValueError("executable permission lost")
    return archive, content


def run(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True, encoding="utf-8").strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--platform", choices=("windows-amd64", "linux-amd64"), required=True)
    parser.add_argument("--release", action="store_true", help="use stable versioned asset names; no publishing")
    args = parser.parse_args()
    binary = args.binary.resolve()
    version = run(str(binary), "version")
    source_version = re.search(r'const Version = "([^"]+)"',
                               (ROOT / "internal/core/core.go").read_text(encoding="utf-8"))[1]
    if version != source_version:
        raise ValueError("binary version differs from source")
    commit = run("git", "rev-parse", "HEAD")
    dirty = bool(run("git", "status", "--porcelain", "--untracked-files=normal"))
    build_info = run("go", "version", "-m", str(binary)) + "\n"
    # A stale binary with the same core version must not claim the current commit.
    validate_build(build_info, commit, args.platform)
    if dirty:
        raise ValueError("commit all source changes before packaging")
    archive, content = assemble(binary, args.output.resolve(), version, commit,
                                args.platform, dirty, build_info,
                                int(run("git", "show", "-s", "--format=%ct", "HEAD")), args.release)
    with tempfile.TemporaryDirectory(prefix="xpotato-version-") as temp:
        shutil.unpack_archive(str(archive), temp)
        name = "xpotato-tunnel.exe" if args.platform.startswith("windows") else "xpotato-tunnel"
        if run(str(Path(temp) / name), "version") != version:
            raise ValueError("extracted binary version differs")
    print(archive)
    print(content)
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as f:
            f.write(f"name=xpotato-tunnel-{version}-{args.platform}-{commit}\n")


if __name__ == "__main__":
    main()
