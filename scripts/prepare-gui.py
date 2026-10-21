"""Prepare a native Go sidecar for a clean GUI build; never runs setup or installs."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tomllib

ROOT=Path(__file__).resolve().parents[1]
TARGETS={'x86_64-pc-windows-msvc':'windows-amd64','x86_64-unknown-linux-gnu':'linux-amd64',
         'x86_64-apple-darwin':'darwin-amd64','aarch64-apple-darwin':'darwin-arm64'}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--core',type=Path,required=True)
    parser.add_argument('--target',choices=TARGETS,required=True)
    args=parser.parse_args()
    commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    if subprocess.check_output(['git','status','--porcelain','--untracked-files=normal'],cwd=ROOT).strip():
        raise ValueError('commit source before preparing a GUI candidate')
    spec=importlib.util.spec_from_file_location('package_review',ROOT/'scripts/package-review.py')
    package=importlib.util.module_from_spec(spec);spec.loader.exec_module(package)
    package.validate_build(subprocess.check_output(['go','version','-m',str(args.core.resolve())],text=True),commit,TARGETS[args.target])
    version=subprocess.check_output([str(args.core.resolve()),'version'],text=True).strip()
    cargo=tomllib.loads((ROOT/'gui/src-tauri/Cargo.toml').read_text())
    if cargo['package']['version']!=version:raise ValueError('GUI/core version mismatch')
    suffix='.exe' if args.target.endswith('windows-msvc') else ''
    destination=ROOT/'gui/src-tauri/binaries'/f'xpotato-tunnel-{args.target}{suffix}'
    destination.parent.mkdir(exist_ok=True)
    shutil.copy2(args.core,destination)
    if suffix=='':destination.chmod(0o755)
    # GitHub reads these literal, validated hash/boolean values as build env.
    if os.environ.get('GITHUB_ENV'):
        with open(os.environ['GITHUB_ENV'],'a',encoding='utf-8') as out:
            out.write(f'XPOTATO_BUILD_COMMIT={commit}\nXPOTATO_BUILD_CLEAN=1\n')
    print(json.dumps(dict(commit=commit,version=version,target=args.target,sidecar=str(destination))))

if __name__=='__main__':main()
