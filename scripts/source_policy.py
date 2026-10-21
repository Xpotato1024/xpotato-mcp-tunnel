"""One target policy for source generation, audit and offline rebuild. No app logic."""
import argparse
import fnmatch
import hashlib
import json
import os
import re
from pathlib import Path, PurePosixPath
import subprocess
import tomllib

ROOT=Path(__file__).resolve().parents[1]
STUB=b'// Xpotato generated resolution-only stub; no upstream implementation.\ncompile_error!("This dependency is excluded from the Windows/Linux source distribution");\n'
STUB_HEADER='# Xpotato resolution-only stub; upstream implementation is not distributed.\n'
STUB_FILES={'Cargo.toml','src/lib.rs','.cargo-checksum.json'}

def long_path(path):
    """Use the Windows file API's explicit long-path form; no OS policy change."""
    path=path.absolute()
    if os.name=='nt' and not str(path).startswith('\\\\?\\'):
        return Path('\\\\?\\UNC\\'+str(path)[2:]) if str(path).startswith('\\\\') else Path('\\\\?\\'+str(path))
    return path

def profile(name, root=ROOT):
    records=json.loads((root/'packaging/source-profiles.json').read_text(encoding='utf-8'))
    if name not in records:raise ValueError('unknown source profile')
    return dict(records[name],name=name)

def digest(path):
    h=hashlib.sha256()
    with long_path(path).open('rb') as f:
        for block in iter(lambda:f.read(65536),b''):h.update(block)
    return h.hexdigest()

def excluded(name, policy):
    return any(fnmatch.fnmatchcase(name,p) for p in policy['excludedBindingNames'])

def selected(metadata):
    """Reachable graph, not Cargo's package catalog (which can contain other OSes)."""
    packages={p['id']:p for p in metadata['packages']}
    nodes={n['id']:n for n in metadata['resolve']['nodes']}
    pending=[metadata['resolve']['root']]; seen=set()
    while pending:
        key=pending.pop()
        if key in seen:continue
        if key not in nodes or key not in packages:raise ValueError('incomplete Cargo graph')
        seen.add(key);pending.extend(d['pkg'] for d in nodes[key]['deps'])
    return {key:packages[key] for key in seen}

def metadata(root, target):
    return json.loads(subprocess.check_output(['cargo','metadata','--frozen','--all-features','--format-version','1',
        '--filter-platform',target,'--manifest-path','gui/src-tauri/Cargo.toml'],cwd=root))

def graph(root, policy):
    result={}
    for target in policy['targets']:
        for key,p in selected(metadata(root,target)).items():
            if excluded(p['name'],policy):raise ValueError('excluded binding is reachable: '+p['name'])
            result[key]=p
    return result

def files(directory):
    directory=long_path(directory)
    result={}
    for p in directory.rglob('*'):
        if p.is_symlink():raise ValueError('source tree contains symlink')
        if p.is_file():result[p.relative_to(directory).as_posix()]=p
    return result

def portable_paths(names):
    """Reject case-only file or directory aliases before a Windows extraction."""
    seen={}
    for name in names:
        parts=PurePosixPath(name).parts
        for i in range(1,len(parts)+1):
            prefix='/'.join(parts[:i]);folded=prefix.casefold()
            if folded in seen and seen[folded]!=prefix:raise ValueError('case-colliding source paths: '+seen[folded]+' / '+prefix)
            seen[folded]=prefix

def normalize_readmes(root):
    """Keep identical README bytes once at the manifest spelling; never alter code."""
    records=[]
    for directory in sorted((root/'gui/vendor').iterdir()):
        manifest=tomllib.loads((directory/'Cargo.toml').read_text(encoding='utf-8'))
        canonical=manifest['package'].get('readme')
        if not isinstance(canonical,str) or not re.fullmatch(r'readme(?:\.[a-z0-9]+)?',canonical,re.I):continue
        candidates=[p for p in directory.iterdir() if p.is_file() and p.name.casefold()==canonical.casefold()]
        aliases=sorted(p.name for p in candidates if p.name!=canonical)
        if not aliases:continue
        checks_path=directory/'.cargo-checksum.json';checks=json.loads(checks_path.read_text(encoding='utf-8'))
        checksum_files(directory,checks)
        hashes={digest(p) for p in candidates}
        if len(hashes)!=1:raise ValueError('different README bytes at case-colliding paths: '+directory.name)
        checksum=hashes.pop();exact=next((p for p in candidates if p.name==canonical),None)
        for path in candidates:
            if path.name==canonical:continue
            if exact is None:path.rename(directory/canonical);exact=directory/canonical
            else:path.unlink()
        for alias in aliases:checks['files'].pop(alias)
        checks['files'][canonical]=checksum
        checks_path.write_text(json.dumps(checks,sort_keys=True)+'\n',encoding='utf-8',newline='\n')
        records.append(dict(package=manifest['package']['name'],version=manifest['package']['version'],
            canonical=canonical,identicalAliases=aliases,sha256=checksum))
    return records

def checksum_files(directory, checksums):
    actual=files(directory)
    if set(actual)!=(set(checksums['files'])|{'.cargo-checksum.json'}):raise ValueError('vendor inventory mismatch: '+directory.name)
    for name,expected in checksums['files'].items():
        path=PurePosixPath(name)
        if path.is_absolute() or '..' in path.parts or '\\' in name:raise ValueError('unsafe vendor checksum path')
        if digest(actual[name])!=expected:raise ValueError('vendor file checksum mismatch: '+name)

def audit_vendor(root, policy):
    """Fail closed on extra stub files, active stubs, missing source or SDK output."""
    active=graph(root,policy)
    used={(p['name'],p['version']) for p in active.values() if p['source']}
    locked={(p['name'],p['version']):p for p in tomllib.loads((root/'gui/src-tauri/Cargo.lock').read_text(encoding='utf-8'))['package'] if p.get('source')}
    found=set(); kept=[]; stubs=[]
    for directory in sorted((root/'gui/vendor').iterdir()):
        if directory.is_symlink() or not directory.is_dir():raise ValueError('unexpected vendor entry')
        manifest=tomllib.loads((directory/'Cargo.toml').read_text(encoding='utf-8'))
        identity=(manifest['package']['name'],manifest['package']['version']);name,version=identity
        if identity in found or identity not in locked:raise ValueError('unexpected/duplicate vendor package')
        found.add(identity)
        checks=json.loads((directory/'.cargo-checksum.json').read_text(encoding='utf-8'))
        if checks['package']!=locked[identity]['checksum']:raise ValueError('vendor registry pin mismatch')
        checksum_files(directory,checks)
        stub=(directory/'src/lib.rs').is_file() and (directory/'src/lib.rs').read_bytes()==STUB
        if stub:
            if set(files(directory))!=STUB_FILES or not (directory/'Cargo.toml').read_text(encoding='utf-8').startswith(STUB_HEADER):raise ValueError('invalid resolution stub')
            if identity in used:raise ValueError('active dependency replaced with stub')
            stubs.append(dict(name=name,version=version,registryChecksum=checks['package']))
        else:
            if excluded(name,policy):raise ValueError('excluded binding implementation remains: '+name)
            for path in files(directory).values():
                if path.suffix=='.rs' and b"generated by `objc2`'s `header-translator`" in path.read_bytes():
                    raise ValueError('generated Apple binding found outside stub')
            kept.append(dict(name=name,version=version,registryChecksum=checks['package']))
    if found!=set(locked):raise ValueError('missing locked resolution metadata/source')
    if not used.issubset(found):raise ValueError('missing active source')
    return dict(profile=policy['name'],targets=policy['targets'],heldTargets=policy['heldTargets'],
        implementationPackages=kept,resolutionOnlyStubs=stubs,lockSHA256=digest(root/'gui/src-tauri/Cargo.lock'))

def harden_stubs(root):
    """Filterer removes code; our stubs deliberately refuse accidental compilation."""
    for directory in sorted((root/'gui/vendor').iterdir()):
        if set(files(directory))!=STUB_FILES or (directory/'src/lib.rs').read_bytes()!=b'':continue
        manifest=directory/'Cargo.toml'
        manifest.write_text(STUB_HEADER+manifest.read_text(encoding='utf-8'),encoding='utf-8',newline='\n')
        (directory/'src/lib.rs').write_bytes(STUB)
        checks_path=directory/'.cargo-checksum.json';checks=json.loads(checks_path.read_text(encoding='utf-8'))
        checks['files']={name:digest(directory/name) for name in ('Cargo.toml','src/lib.rs')}
        checks_path.write_text(json.dumps(checks,sort_keys=True)+'\n',encoding='utf-8',newline='\n')

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile',default='windows-linux');parser.add_argument('--target',required=True)
    parser.add_argument('--github-output',type=Path,required=True)
    args=parser.parse_args();p=profile(args.profile)
    if args.target not in p['targets']+p['heldTargets']:raise ValueError('undeclared distribution target')
    with args.github_output.open('a',encoding='utf-8',newline='\n') as out:
        out.write('eligible='+str(args.target in p['targets']).lower()+'\nprofile='+args.profile+'\n')

if __name__=='__main__':main()
