"""Build a local NSIS candidate from an already checked package. Never installs/publishes."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    hasher = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def nsis_literal(value):
    # Do not allow arbitrary compiler instructions in generated include files.
    if any(c in str(value) for c in '$"\r\n\x00'):
        raise ValueError("unsupported NSIS path")
    return str(value)


def generate_includes(payload, output):
    files = sorted(p for p in payload.rglob("*") if p.is_file())
    if not files or len(files) > 128 or any(p.is_symlink() for p in payload.rglob("*")):
        raise ValueError("unexpected installer payload")
    install, remove, cleanup = [], [], []
    for index, path in enumerate(files):
        relative = path.relative_to(payload)
        name = nsis_literal(str(relative).replace("/", "\\"))
        parent = str(relative.parent).replace("/", "\\")
        suffix = "" if parent == "." else "\\" + nsis_literal(parent)
        install += [f'  SetOutPath "$StagingDir{suffix}"', '  IfErrors invalid',
                    f'  File "{nsis_literal(path)}"', '  IfErrors invalid']
        cleanup.append(f'  Delete "$StagingDir\\{name}"')
        if name != "xpotato-tunnel.exe":
            remove += [f'  IfFileExists "$INSTDIR\\{name}" 0 removed_{index}',
                       '  ClearErrors', f'  Delete "$INSTDIR\\{name}"', '  IfErrors blocked', f'removed_{index}:']
    directories = {p.parent for p in files}
    for path in list(directories):
        while path != payload:
            directories.add(path)
            path = path.parent
    for path in sorted(directories, key=lambda p: len(p.parts), reverse=True):
        if path != payload:
            remove.append(f'  RMDir "$INSTDIR\\{nsis_literal(str(path.relative_to(payload)))}"')
            cleanup.append(f'  RMDir "$StagingDir\\{nsis_literal(str(path.relative_to(payload)))}"')
    checks = ['!macro CheckPayloadDirectories base failure',
              '  !insertmacro CheckDirectory "${base}" ${failure}']
    for path in sorted(directories, key=lambda p: (len(p.parts), str(p))):
        if path != payload:
            relative = nsis_literal(str(path.relative_to(payload)).replace("/", "\\"))
            checks.append('  !insertmacro CheckDirectory "${base}\\' + relative + '" ${failure}')
    checks.append('!macroend')
    (output / "check-directories.nsh").write_text("\n".join(checks) + "\n", encoding="utf-8")
    for name, lines in (("install-files.nsh", install), ("uninstall-files.nsh", remove), ("cleanup-stage.nsh", cleanup)):
        (output / name).write_text("\n".join(lines) + "\n", encoding="utf-8")


def gui_defines(payload, info):
    gui=payload/'xpotato-tunnel-app.exe'
    if not gui.exists():return []
    gui_info=json.loads(subprocess.check_output([str(gui),'--build-info'],text=True))
    if gui_info.get('nativeFixture') is not False:raise ValueError('non-distributable GUI fixture or missing production marker')
    if gui_info['dirty'] or gui_info['commit']!=info['commit'] or gui_info['version']!=info['version'] or gui_info['coreSHA256']!=digest(payload/'xpotato-tunnel.exe'):
        raise ValueError('GUI/core provenance mismatch')
    return ['/DGUI_SHA='+digest(gui)]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--content", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--makensis", required=True, type=Path)
    parser.add_argument("--compiler-sha256", required=True)
    parser.add_argument("--nsis-license", required=True, type=Path)
    args = parser.parse_args()
    pin = json.loads((ROOT / "installer/nsis-toolchain.json").read_text(encoding="utf-8"))
    if args.compiler_sha256 != pin["compilerSHA256"] or digest(args.makensis) != pin["compilerSHA256"]:
        raise ValueError("compiler differs from the separately reviewed pinned toolchain")
    if digest(args.nsis_license) != pin["licenseSHA256"]:
        raise ValueError("NSIS license differs from the reviewed distribution")
    spec = importlib.util.spec_from_file_location("package_review", ROOT / "scripts/package-review.py")
    package = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(package)
    package.verify(args.content)
    info = json.loads((args.content / "build-info.json").read_text(encoding="utf-8"))
    if info["platform"] != "windows-amd64" or info["dirty"] or not re.fullmatch(r"\d+\.\d+\.\d+", info["version"]):
        raise ValueError("expected clean Windows payload")
    binary = args.content.resolve() / "xpotato-tunnel.exe"
    if subprocess.check_output([str(binary), "version"], text=True).strip() != info["version"]:
        raise ValueError("installer payload version differs from executable")
    source = json.loads(subprocess.check_output([str(binary), "source"], text=True))
    if source["commit"] != info["commit"] or source["modified"]:
        raise ValueError("installer executable provenance differs from payload")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    payload = output / "payload"
    shutil.copytree(args.content, payload)
    # Retain the reviewed compiler distribution's actual license text. zlib
    # compression avoids adding the LZMA component; no optional plugins used.
    shutil.copyfile(args.nsis_license, payload / "licenses/NSIS-LICENSE.txt")
    with (payload / "SHA256SUMS").open("a", encoding="utf-8", newline="\n") as stream:
        stream.write(f"{digest(payload / 'licenses/NSIS-LICENSE.txt')}  licenses/NSIS-LICENSE.txt\n")
    generate_includes(payload, output)
    gui_flags=gui_defines(payload,info)
    script = output / "windows.nsi"
    shutil.copyfile(ROOT / "installer/windows.nsi", script)
    installer = output / f"xpotato-tunnel-{info['version']}-windows-amd64-setup.exe"
    import os
    compiler_env = {key: value for key, value in os.environ.items() if key.upper() not in ("NSISDIR", "NSISCONFDIR")}
    subprocess.run([str(args.makensis.resolve()), "/NOCONFIG", "/INPUTCHARSET", "UTF8",
                    "/DVERSION=" + info["version"], "/DPAYLOAD=" + nsis_literal(payload),
                    "/DCORE_SHA=" + digest(payload / "xpotato-tunnel.exe"),
                    "/DOUTPUT=" + nsis_literal(installer), *gui_flags, str(script)], cwd=output, env=compiler_env, check=True)
    (output / (installer.name + ".sha256")).write_text(
        f"{digest(installer)}  {installer.name}\n", encoding="utf-8", newline="\n")
    print(installer)


if __name__ == "__main__":
    main()
