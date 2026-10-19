"""Assemble native review/release candidates locally. Never uploads or tags."""

import argparse
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
        "runtime-ux.md", "performance-review.md", "distribution.md")


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


def assemble(binary, output, version, commit, platform, dirty, build_info):
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
    shutil.copytree(ROOT / "docs/performance-results", content / "performance-results")
    shutil.copy2(ROOT / "setup.env.example", content / "setup.env.example")
    (content / "build-info.txt").write_text(build_info, encoding="utf-8", newline="\n")
    metadata = dict(version=version, commit=commit, platform=platform, dirty=dirty,
                    runtimeIncluded=False, binary=name)
    (content / "build-info.json").write_text(
        json.dumps(metadata, indent=2) + "\n", encoding="utf-8", newline="\n")
    files = sorted(p for p in content.rglob("*") if p.is_file())
    (content / "SHA256SUMS").write_text("".join(
        f"{sha256(p)}  {p.relative_to(content).as_posix()}\n" for p in files),
        encoding="utf-8", newline="\n")
    stem = f"xpotato-tunnel-{version}-{platform}-{commit}"
    archive = output / (stem + (".zip" if platform.startswith("windows") else ".tar.gz"))
    if platform.startswith("windows"):
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as z:
            for p in sorted(content.rglob("*")):
                if p.is_file():
                    z.write(p, p.relative_to(content).as_posix())
    else:
        with tarfile.open(archive, "w:gz") as tar:
            for p in sorted(content.rglob("*")):
                if p.is_file():
                    def mode(info):
                        info.mode = 0o755 if info.name == name else 0o644
                        return info
                    tar.add(p, arcname=p.relative_to(content).as_posix(), filter=mode)
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
                                args.platform, dirty, build_info)
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
