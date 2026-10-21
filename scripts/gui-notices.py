"""Collect exact Cargo package license/notice text into one build-time document."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[1]

def override_text(package, directory):
    """A few crates omit license text; bind reviewed texts to their source commit."""
    base=ROOT/'gui/licenses'
    records=json.loads((base/'overrides.json').read_text(encoding='utf-8'))
    record=records.get(package['name']+'@'+package['version'])
    if not record:return None
    vcs=json.loads((directory/'.cargo_vcs_info.json').read_text(encoding='utf-8'))
    if record['license']!=package['license'] or record['commit']!=vcs['git']['sha1']:
        raise ValueError('license override source differs')
    texts=[]
    if record.get('selectedLicense'):
        if record['selectedLicense'] not in record['license'].split(' OR '):raise ValueError('invalid selected license')
        texts.append('Selected license: '+record['selectedLicense']+'\n'+record.get('notes',''))
    for entry in record['files']:
        path=base/entry['path']
        if path.parent!=base or path.is_symlink() or not path.is_file():raise ValueError('invalid override path')
        data=path.read_bytes()
        if hashlib.sha256(data).hexdigest()!=entry['sha256']:raise ValueError('license override digest differs')
        texts.append(entry['url']+'\n'+data.decode('utf-8')+'\n')
    return '\n'.join(texts)

def collect(metadata):
    sections=['Xpotato Tunnel GUI third-party notices\nProject code: AGPL-3.0-only; see LICENSE.\n']
    missing=[]
    for package in sorted(metadata['packages'],key=lambda p:(p['name'],p['version'])):
        if package['id']==metadata['resolve']['root']:continue
        directory=Path(package['manifest_path']).resolve().parent
        files=set()
        for pattern in ('LICENSE*','license*','LICENCE*','COPYING*','NOTICE*','COPYRIGHT*'):
            files.update(p for p in directory.glob(pattern) if p.is_file() and not p.is_symlink())
        if package.get('license_file'):
            path=(directory/package['license_file']).resolve()
            if directory not in path.parents or not path.is_file() or path.is_symlink():raise ValueError('invalid package license path')
            files.add(path)
        extra=None
        if not files:
            extra=override_text(package,directory)
            if not extra:missing.append(package['name']+'@'+package['version']);continue
        sections.append(f"\n--- {package['name']} {package['version']} ---\nSPDX: {package.get('license')}\nSource: {package.get('repository') or package.get('source')}\n")
        sections.append('Package authors (Cargo metadata): '+', '.join(package.get('authors',[]))+'\n')
        if extra:sections.append(extra)
        for path in sorted(files):
            if path.stat().st_size>1024*1024:raise ValueError('unexpected license size')
            sections.append(path.name+'\n'+path.read_text(encoding='utf-8')+'\n')
    if missing:raise ValueError('license text missing: '+', '.join(missing))
    return '\n'.join(sections)

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--target',required=True)
    args=parser.parse_args()
    data=subprocess.check_output(['cargo','metadata','--locked','--format-version','1','--filter-platform',args.target,'--manifest-path',str(ROOT/'gui/src-tauri/Cargo.toml')],cwd=ROOT)
    notice=collect(json.loads(data))
    args.output.parent.mkdir(parents=True,exist_ok=True)
    with args.output.open('x',encoding='utf-8',newline='\n') as out:out.write(notice)

if __name__=='__main__':main()
