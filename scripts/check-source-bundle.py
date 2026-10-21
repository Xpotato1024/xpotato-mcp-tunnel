"""CI exercises the delivered archive's own rebuild entrypoint, not the checkout."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tarfile
from source_policy import digest, long_path

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--archive',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    p.add_argument('--target',required=True);p.add_argument('--expected-commit',required=True)
    a=p.parse_args();archive=a.archive.resolve();out=a.output.resolve()
    expected=archive.with_name(archive.name+'.sha256').read_text(encoding='utf-8').split()[0]
    if digest(archive)!=expected:raise ValueError('source archive digest mismatch')
    out.mkdir(parents=True,exist_ok=False);root=out/'source';root.mkdir()
    with tarfile.open(archive) as tar:
        entries=tar.getmembers()
        if len(entries)>150000 or sum(m.size for m in entries)>1024*1024*1024 or any(not m.isfile() for m in entries):raise ValueError('unexpected source archive')
        names=[m.name for m in entries]
        if len(names)!=len(set(names)):raise ValueError('duplicate source path')
        tar.extractall(long_path(root),filter='data')
    info=json.loads((root/'SOURCE-PROVENANCE.json').read_text(encoding='utf-8'))
    if info['commit']!=a.expected_commit:raise ValueError('source archive head mismatch')
    subprocess.run([sys.executable,str(root/'scripts/rebuild-source.py'),'--target',a.target,'--output',str(out/'build')],cwd=root,check=True)

if __name__=='__main__':main()
