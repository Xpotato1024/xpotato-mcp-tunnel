"""Create a clean, deterministic corresponding-source candidate. Never publishes."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import sys
sys.dont_write_bytecode = True # Dynamic imports must not write into the source snapshot.
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'scripts'))
from source_policy import long_path, portable_paths

def run(*args):
    return subprocess.check_output(args, cwd=ROOT)

def pack(content, archive, epoch):
    content=long_path(content)
    if any((content/'scripts').rglob('__pycache__')):
        raise ValueError('generated script bytecode must not enter corresponding source')
    files = sorted(p for p in content.rglob('*') if p.is_file())
    portable_paths(p.relative_to(content).as_posix() for p in files)
    if any(p.is_symlink() for p in content.rglob('*')):
        raise ValueError('source bundle must contain no symlinks')
    checksums = ''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.relative_to(content).as_posix()}\n'
                        for p in files if p != content/'SOURCE-SHA256SUMS')
    (content/'SOURCE-SHA256SUMS').write_text(checksums, encoding='utf-8', newline='\n')
    with archive.open('xb') as raw, gzip.GzipFile(filename='',fileobj=raw,mode='wb',mtime=epoch) as compressed:
        with tarfile.open(fileobj=compressed, mode='w') as tar:
            for path in sorted(p for p in content.rglob('*') if p.is_file()):
                info = tarfile.TarInfo(path.relative_to(content).as_posix())
                info.mode = 0o755 if path.stat().st_mode & 0o111 else 0o644
                info.mtime = epoch
                info.size = path.stat().st_size
                with path.open('rb') as source: tar.addfile(info, source)
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    archive.with_name(archive.name+'.sha256').write_text(f'{digest}  {archive.name}\n',encoding='utf-8',newline='\n')

def main():
    if sys.version_info < (3,12):
        raise SystemExit('source packaging requires Python 3.12 or later (safe tar extraction)')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--profile',default='core',choices=['core',*json.loads((ROOT/'packaging/source-profiles.json').read_text(encoding='utf-8'))])
    parser.add_argument('--vendor-filterer',type=Path,help='Pinned developer filterer; required for GUI source')
    args = parser.parse_args()
    if (args.profile!='core')!=bool(args.vendor_filterer):raise ValueError('GUI source requires an explicit profile and pinned filterer')
    if run('git','status','--porcelain','--untracked-files=normal').strip():
        raise ValueError('commit all source changes before packaging')
    commit = run('git','rev-parse','HEAD').decode().strip()
    epoch = int(run('git','show','-s','--format=%ct','HEAD'))
    version = re.search(r'const Version = "([^"]+)"',(ROOT/'internal/core/core.go').read_text(encoding='utf-8'))[1]
    toolchain = run('go','version').decode().strip()
    if not re.search(r'\bgo1\.27\.2\b',toolchain):
        raise ValueError('use the reviewed Go 1.27.2 toolchain')
    output = args.output.resolve()
    output.mkdir(parents=True,exist_ok=False)
    with tempfile.TemporaryDirectory(prefix='xpotato-source-') as temp:
        content = Path(temp)/'source'
        content.mkdir()
        data = run('git','archive','--format=tar',commit)
        with tarfile.open(fileobj=io.BytesIO(data),mode='r:') as tar:
            tar.extractall(content,filter='data')
        subprocess.run(['go','mod','verify'],cwd=ROOT,check=True)
        subprocess.run(['go','mod','vendor','-o',str(content/'vendor')],cwd=ROOT,check=True)
        filtering=None
        if args.profile!='core':
            import importlib.util
            spec=importlib.util.spec_from_file_location('source_vendor',ROOT/'scripts/source-vendor.py')
            module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
            filtering=module.vendor(content,args.profile,args.vendor_filterer.resolve())
        if run('git','rev-parse','HEAD').decode().strip()!=commit or run('git','status','--porcelain','--untracked-files=normal').strip():
            raise ValueError('source checkout changed while collecting dependencies')
        metadata = dict(commit=commit,version=version,license='AGPL-3.0-only',toolchain=toolchain,
                        sourceDateEpoch=epoch,goDependenciesIncluded=True,runtimeIncluded=False,
                        buildInstructions='docs/licensing.md')
        metadata['rustDependenciesIncluded']=bool(filtering)
        metadata['sourceProfile']=args.profile
        if filtering:
            metadata['targets']=filtering['targets']
            metadata['heldTargets']=filtering['heldTargets']
            metadata['cargoLockSHA256']=filtering['lockSHA256']
            metadata['buildInstructions']='docs/source-distribution.md'
        (content/'SOURCE-PROVENANCE.json').write_text(json.dumps(metadata,indent=2)+'\n',encoding='utf-8',newline='\n')
        archive = output/f'xpotato-tunnel-{version}-source.tar.gz'
        pack(content,archive,epoch)
        print(archive)

if __name__ == '__main__': main()
