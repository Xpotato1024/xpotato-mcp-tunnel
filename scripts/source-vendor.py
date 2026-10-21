"""Generate target-filtered Rust sources; never copies a caller's unaudited vendor tree."""
import argparse
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import urllib.request
from source_policy import ROOT, profile, graph, harden_stubs, audit_vendor

def install_tool(output):
    pin=profile('windows-linux')['filterer'];version=pin['version']
    with urllib.request.urlopen(f'https://static.crates.io/crates/cargo-vendor-filterer/cargo-vendor-filterer-{version}.crate',timeout=60) as response:
        data=response.read(1024*1024+1)
    if len(data)>1024*1024 or hashlib.sha256(data).hexdigest()!=pin['archiveSHA256']:raise ValueError('filterer source pin mismatch')
    output.mkdir(parents=True,exist_ok=False)
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        entries=archive.getmembers()
        if len(entries)>200 or sum(e.size for e in entries)>4*1024*1024 or any(not(e.isfile() or e.isdir()) for e in entries):raise ValueError('unexpected filterer archive')
        archive.extractall(output/'source',filter='data')
    subprocess.run(['cargo','install','--path',str(output/'source'/f'cargo-vendor-filterer-{version}'),'--locked','--root',str(output),'-j','2'],check=True)

def vendor(root, profile_name, tool):
    policy=profile(profile_name,root)
    if subprocess.check_output([str(tool),'--version'],text=True).strip()!='cargo-vendor-filterer '+policy['filterer']['version']:raise ValueError('unexpected filterer version')
    lock=root/'gui/src-tauri/Cargo.lock';original=lock.read_bytes()
    subprocess.run(['cargo','fetch','--locked','--manifest-path','gui/src-tauri/Cargo.toml'],cwd=root,check=True)
    before={(p['name'],p['version']) for p in graph(root,policy).values()}
    command=[str(tool),'--offline','--all-features','--versioned-dirs','--manifest-path','gui/src-tauri/Cargo.toml']
    for target in policy['targets']:command.extend(['--platform',target])
    subprocess.run([*command,'gui/vendor'],cwd=root,check=True)
    if lock.read_bytes()!=original:raise ValueError('filterer changed the locked dependency graph')
    harden_stubs(root)
    (root/'.cargo').mkdir(exist_ok=True)
    (root/'.cargo/config.toml').write_text('[net]\noffline = true\n[source.crates-io]\nreplace-with = "vendored-sources"\n[source.vendored-sources]\ndirectory = "gui/vendor"\n',encoding='utf-8',newline='\n')
    after=graph(root,policy)
    if before!={(p['name'],p['version']) for p in after.values()}:raise ValueError('active graph changed during filtering')
    report=audit_vendor(root,policy)
    report['filterer']=policy['filterer']
    (root/'SOURCE-FILTERING.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8',newline='\n')
    # Preserve the complete license/NOTICE texts of all actual build dependencies.
    spec=importlib.util.spec_from_file_location('gui_notices',root/'scripts/gui-notices.py')
    notices=importlib.util.module_from_spec(spec);spec.loader.exec_module(notices)
    root_id=next(p['id'] for p in after.values() if p['source'] is None)
    text=notices.collect(dict(packages=list(after.values()),resolve=dict(root=root_id)))
    (root/'SOURCE-THIRD-PARTY-NOTICES.txt').write_text(text,encoding='utf-8',newline='\n')
    return report

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--install-tool',type=Path,required=True)
    args=parser.parse_args();install_tool(args.install_tool.resolve())

if __name__=='__main__':main()
