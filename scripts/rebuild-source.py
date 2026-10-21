"""Rebuild this unmodified source bundle with empty dependency caches and no registry access."""
import argparse
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
sys.dont_write_bytecode = True # Keep the delivered source inventory unchanged on import.
from source_policy import ROOT, profile, digest, audit_vendor, long_path

def verify_source(root):
    root=long_path(root)
    expected={}
    for line in (root/'SOURCE-SHA256SUMS').read_text(encoding='utf-8').splitlines():
        checksum,name=line.split('  ',1);path=PurePosixPath(name)
        if not re.fullmatch('[0-9a-f]{64}',checksum) or name in expected or path.is_absolute() or '..' in path.parts or '\\' in name:raise ValueError('invalid source manifest')
        expected[name]=checksum
    actual={p.relative_to(root).as_posix():p for p in root.rglob('*') if p.is_file()}
    if any(p.is_symlink() for p in root.rglob('*')):raise ValueError('source symlink rejected')
    if set(actual)!=(set(expected)|{'SOURCE-SHA256SUMS'}):
        missing=sorted(set(expected)-set(actual));extra=sorted(set(actual)-set(expected)-{'SOURCE-SHA256SUMS'})
        raise ValueError(f'source inventory changed; missing={missing[:8]} extra={extra[:8]}; use a separate fork build/provenance')
    for name,checksum in expected.items():
        if digest(actual[name])!=checksum:raise ValueError('source changed: '+name)

def rebuild(root,target,output,tests=True):
    verify_source(root)
    provenance=json.loads((root/'SOURCE-PROVENANCE.json').read_text(encoding='utf-8'))
    policy=profile(provenance['sourceProfile'],root)
    if target not in policy['targets']:raise ValueError('target is held or unsupported by this source profile')
    if not re.fullmatch('[0-9a-f]{40}',provenance['commit']):raise ValueError('invalid source provenance')
    if digest(root/'gui/src-tauri/Cargo.lock')!=provenance['cargoLockSHA256']:raise ValueError('source lock mismatch')
    report=audit_vendor(root,policy)
    host=re.search(r'^host: (.+)$',subprocess.check_output(['rustc','-vV'],text=True),re.M)[1]
    if target!=host:raise ValueError('use the matching native host/SDK; cross compilation is not certified')
    if subprocess.check_output(['rustc','--version'],text=True).split()[1]!='1.94.1':raise ValueError('use Rust 1.94.1')
    if not re.search(r'\bgo1\.27\.3\b',subprocess.check_output(['go','version'],text=True,env=dict(os.environ,GOTOOLCHAIN='local'))):raise ValueError('use Go 1.27.3')
    output.mkdir(parents=True,exist_ok=False)
    env=dict(os.environ,GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',
        GOMODCACHE=str(output/'go-mod-empty'),GOCACHE=str(output/'go-cache'),
        CARGO_HOME=str(output/'cargo-home-empty'),CARGO_TARGET_DIR=str(output/'cargo-target'),
        CARGO_NET_OFFLINE='true',XPOTATO_BUILD_COMMIT=provenance['commit'],XPOTATO_BUILD_CLEAN='1',
        XPOTATO_RUNTIME_SMOKE='0',GOMAXPROCS='2',CARGO_BUILD_JOBS='2')
    (output/'cargo-home-empty').mkdir();(output/'go-mod-empty').mkdir()
    def run(*command):subprocess.run(command,cwd=root,env=env,check=True)
    suffix='.exe' if target.endswith('windows-msvc') else ''
    core=output/('xpotato-tunnel'+suffix)
    run('go','build','-mod=vendor','-buildvcs=false','-trimpath','-ldflags',
        '-X github.com/Xpotato1024/xpotato-mcp-tunnel/internal/licensing.BuildCommit='+provenance['commit'],
        '-o',str(core),'./cmd/xpotato-tunnel')
    binary=root/'gui/src-tauri/binaries'/('xpotato-tunnel-'+target+suffix)
    binary.parent.mkdir(exist_ok=True);shutil.copy2(core,binary)
    cargo=['cargo','--frozen','--manifest-path','gui/src-tauri/Cargo.toml','--release','-j','2']
    run(cargo[0],'build',*cargo[1:])
    if tests:
        run('go','test','-mod=vendor','-p','2','-count=1','./...')
        run(cargo[0],'test',*cargo[1:])
    app=output/'cargo-target/release'/('xpotato-tunnel-app'+suffix)
    info=json.loads(subprocess.check_output([str(app),'--build-info'],cwd=root,env=env,text=True))
    if info['nativeFixture'] is not False or info['dirty'] or info['commit']!=provenance['commit'] or info['version']!=provenance['version'] or info['coreSHA256']!=digest(core):raise ValueError('rebuilt GUI provenance mismatch')
    go_info=json.loads(subprocess.check_output([str(core),'source'],cwd=root,env=env,text=True))
    if go_info['commit']!=provenance['commit'] or go_info['modified']:raise ValueError('rebuilt core provenance mismatch')
    result=dict(commit=provenance['commit'],target=target,sourceProfile=provenance['sourceProfile'],
        cargoLockSHA256=report['lockSHA256'],emptyCaches=True,cargoFrozen=True,goProxy='off',gui=info)
    (output/'SOURCE-REBUILD-RESULT.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
    return result

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--target',required=True);p.add_argument('--output',type=Path,required=True)
    a=p.parse_args();print(json.dumps(rebuild(ROOT,a.target,a.output.resolve()),indent=2))

if __name__=='__main__':main()
