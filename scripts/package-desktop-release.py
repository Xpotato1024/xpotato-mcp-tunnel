"""Assemble eight verified Windows/Linux assets. Never creates a tag or publishes."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import tarfile
from source_policy import profile, excluded


def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(65536),b''):h.update(block)
    return h.hexdigest()


def checked_file(path):
    if path.is_symlink() or not path.is_file():raise ValueError('missing or linked candidate: '+str(path))
    return path


def json_file(path):
    checked_file(path)
    if path.stat().st_size>2*1024*1024:raise ValueError('oversized candidate metadata')
    result=json.loads(path.read_text(encoding='utf-8'))
    if not isinstance(result,dict):raise ValueError('expected candidate metadata object')
    return result


def source_metadata(archive):
    # The native jobs already verified the full archive inventory and rebuilt it.
    # Read only the two bounded root records here; do not extract or rescan it.
    wanted={'SOURCE-PROVENANCE.json','SOURCE-FILTERING.json'};records={}
    with tarfile.open(archive,mode='r|gz') as tar:
        for member in tar:
            if member.name not in wanted:continue
            if not member.isfile() or member.size>2*1024*1024 or member.name in records:raise ValueError('invalid source record')
            records[member.name]=json.load(tar.extractfile(member))
            if set(records)==wanted:break
    if set(records)!=wanted:raise ValueError('source records missing')
    return records['SOURCE-PROVENANCE.json'],records['SOURCE-FILTERING.json']


def assemble(inputs,output,commit,ci_url):
    if not re.fullmatch('[0-9a-f]{40}',commit):raise ValueError('invalid commit')
    if not re.fullmatch(r'https://github\.com/[^/]+/[^/]+/actions/runs/[0-9]+',ci_url):raise ValueError('invalid public CI URL')
    policy=profile('windows-linux')
    expected={'source-windows-linux-'+commit,'desktop-windows-amd64-'+commit,'desktop-linux-amd64-'+commit}
    if inputs.is_symlink() or {p.name for p in inputs.iterdir()}!=expected:raise ValueError('unexpected candidate set; Mac and other runs are excluded')
    if any(p.is_symlink() or not p.is_dir() for p in inputs.iterdir()):raise ValueError('invalid candidate directory')
    if any(p.is_symlink() for p in inputs.rglob('*')):raise ValueError('linked candidate input')
    source_dir=inputs/('source-windows-linux-'+commit)
    archives=list(source_dir.glob('*.tar.gz'))
    if len(archives)!=1:raise ValueError('expected one common corresponding source archive')
    source=checked_file(archives[0]);info,filtering=source_metadata(source)
    version=info['version']
    if not re.fullmatch(r'\d+\.\d+\.\d+',version):raise ValueError('invalid version')
    if source.name!=f'xpotato-tunnel-{version}-source.tar.gz':raise ValueError('source filename/version mismatch')
    if info['commit']!=commit or info['sourceProfile']!='windows-linux' or not info['rustDependenciesIncluded'] or not info['goDependenciesIncluded']:raise ValueError('wrong source commit/profile')
    if info['targets']!=policy['targets'] or info['heldTargets']!=policy['heldTargets']:raise ValueError('source target policy mismatch')
    if filtering['profile']!='windows-linux' or filtering['targets']!=policy['targets'] or filtering['heldTargets']!=policy['heldTargets'] or filtering['lockSHA256']!=info['cargoLockSHA256']:raise ValueError('source filtering record mismatch')
    if any(excluded(p['name'],policy) for p in filtering['implementationPackages']):raise ValueError('excluded binding implementation in source report')
    if not re.fullmatch('[0-9a-f]{64}',info['cargoLockSHA256']):raise ValueError('invalid source lock digest')
    primaries=[source];rebuilds=[]
    for platform,target,extension in [('windows-amd64','x86_64-pc-windows-msvc','-setup.exe'),('linux-amd64','x86_64-unknown-linux-gnu','.deb')]:
        directory=inputs/('desktop-'+platform+'-'+commit)
        result=json_file(directory/'source-rebuild/build/SOURCE-REBUILD-RESULT.json')
        gui=result['gui']
        if result['commit']!=commit or result['target']!=target or result['sourceProfile']!='windows-linux' or result['cargoLockSHA256']!=info['cargoLockSHA256']:raise ValueError('native rebuild source mismatch')
        if result['emptyCaches'] is not True or result['cargoFrozen'] is not True or result['goProxy']!='off':raise ValueError('offline/empty-cache rebuild gate missing')
        if gui['commit']!=commit or gui['version']!=version or gui['dirty'] is not False or gui['nativeFixture'] is not False or not re.fullmatch('[0-9a-f]{64}',gui['coreSHA256']):raise ValueError('non-production or mismatched rebuild')
        package=directory/('installer' if platform.startswith('windows') else 'gui')/f'xpotato-tunnel-{version}-{platform}{extension}'
        primaries.append(checked_file(package));rebuilds.append(result)
    assets=[]
    for path in primaries:
        sha=digest(path);sidecar=checked_file(path.with_name(path.name+'.sha256'))
        if path.stat().st_size==0 or sidecar.stat().st_size>512:raise ValueError('invalid candidate size')
        if sidecar.read_text(encoding='utf-8').strip()!=sha+'  '+path.name:raise ValueError('candidate digest mismatch: '+path.name)
        assets.append(dict(name=path.name,sha256=sha,bytes=path.stat().st_size))
    # No output before all inputs and proofs pass. Never overwrite prior evidence.
    output.mkdir(parents=True,exist_ok=False)
    for path in primaries:
        shutil.copy2(path,output/path.name);shutil.copy2(path.with_name(path.name+'.sha256'),output/(path.name+'.sha256'))
    provenance=dict(project='Xpotato.net / Xpotato1024',version=version,commit=commit,ci=ci_url,
        license='AGPL-3.0-only',sourceProfile='windows-linux',targets=policy['targets'],heldTargets=policy['heldTargets'],
        sourceArchive=source.name,cargoLockSHA256=info['cargoLockSHA256'],
        sourceAudit=dict(implementationPackages=len(filtering['implementationPackages']),resolutionOnlyStubs=len(filtering['resolutionOnlyStubs']),identicalReadmeAliases=filtering.get('identicalReadmeAliases',[])),
        windowsSigning='Unsigned build; this workflow has no signing step.',assets=assets,rebuilds=rebuilds)
    (output/'release-provenance.json').write_text(json.dumps(provenance,indent=2)+'\n',encoding='utf-8',newline='\n')
    (output/'SHA256SUMS.txt').write_text(''.join(digest(p)+'  '+p.name+'\n' for p in sorted(output.iterdir())),encoding='utf-8',newline='\n')
    return provenance


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--input',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    p.add_argument('--commit',required=True);p.add_argument('--ci-url',required=True)
    a=p.parse_args();print(json.dumps(assemble(a.input,a.output,a.commit,a.ci_url),indent=2))


if __name__=='__main__':main()
