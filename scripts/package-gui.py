"""Assemble GUI content and native .app/DMG or .deb candidates. Never installs/publishes/signs."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import plistlib
import re
import shutil
import subprocess

ROOT=Path(__file__).resolve().parents[1]

def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda:source.read(65536),b''):h.update(chunk)
    return h.hexdigest()

def assemble(content,gui,notices,output,platform):
    if platform not in ('windows-amd64','linux-amd64','darwin-amd64','darwin-arm64'):raise ValueError('unsupported GUI platform')
    info=json.loads((content/'build-info.json').read_text())
    if info['platform']!=platform or info['dirty']:raise ValueError('expected clean matching core content')
    spec=importlib.util.spec_from_file_location('package_review',ROOT/'scripts/package-review.py')
    package=importlib.util.module_from_spec(spec);spec.loader.exec_module(package);package.verify(content)
    core_name='xpotato-tunnel.exe' if platform.startswith('windows') else 'xpotato-tunnel'
    gui_info=json.loads(subprocess.check_output([str(gui.resolve()),'--build-info'],text=True))
    if gui_info.get('nativeFixture') is not False:raise ValueError('non-distributable GUI fixture or missing production marker')
    if gui_info['dirty'] or gui_info['commit']!=info['commit'] or gui_info['version']!=info['version'] or gui_info['coreSHA256']!=digest(content/core_name):raise ValueError('GUI/core provenance mismatch')
    if notices.stat().st_size<1000:raise ValueError('missing GUI third-party notice text')
    output.mkdir(parents=True,exist_ok=False)
    payload=output/'content';shutil.copytree(content,payload)
    app_name='xpotato-tunnel-app.exe' if platform.startswith('windows') else 'xpotato-tunnel-app'
    shutil.copy2(gui,payload/app_name)
    shutil.copy2(notices,payload/'licenses/GUI-THIRD-PARTY-LICENSES.txt')
    (payload/'build-info-gui.json').write_text(json.dumps(gui_info,indent=2)+'\n',encoding='utf-8')
    (payload/'README.md').write_text(f'# Xpotato Tunnel {info["version"]}\n\nOpen Xpotato Tunnel from the OS application launcher. Choose Setup, review the allowed folders and storage consent, then enter the API key in the masked field.\n\nChoose Start to connect. Stop affects only this app\'s connection. Update applies at next start; Repair downloads a fresh pinned runtime. Full dependency diagnosis is optional.\n\nApplication updates/removal belong to the OS Installer/package manager. Saved profiles, keys and runtimes are retained.\n\n[License and corresponding source](docs/licensing.md) / [Third-party licenses](licenses/GUI-THIRD-PARTY-LICENSES.txt) / [GUI design](docs/gui-lifecycle.md)\n',encoding='utf-8')
    shutil.copy2(ROOT/'docs/gui-lifecycle.md',payload/'docs/gui-lifecycle.md')
    checksums=''.join(f'{digest(p)}  {p.relative_to(payload).as_posix()}\n' for p in sorted(payload.rglob('*')) if p.is_file() and p!=payload/'SHA256SUMS')
    (payload/'SHA256SUMS').write_text(checksums,encoding='utf-8',newline='\n')
    package.verify(payload)
    return payload,info

def native(payload,info,output):
    platform=info['platform'];version=info['version']
    if not re.fullmatch(r'\d+\.\d+\.\d+',version):raise ValueError('invalid version')
    if platform.startswith('darwin'):
        # Use an explicit image root so the mounted volume contains the .app,
        # rather than depending on how a tool treats a bundle as srcfolder.
        image_root=output/'dmg-root'
        app=image_root/'Xpotato Tunnel.app';mac=app/'Contents/MacOS';resources=app/'Contents/Resources'
        mac.mkdir(parents=True);resources.mkdir()
        for name in ('xpotato-tunnel-app','xpotato-tunnel'):
            shutil.copy2(payload/name,mac/name);(mac/name).chmod(0o755)
        for path in payload.iterdir():
            if path.name in ('xpotato-tunnel-app','xpotato-tunnel','SHA256SUMS'):continue
            if path.is_dir():shutil.copytree(path,resources/path.name)
            else:shutil.copy2(path,resources/path.name)
        shutil.copy2(ROOT/'gui/src-tauri/icons/icon.icns',resources/'icon.icns')
        (app/'Contents/Info.plist').write_bytes(plistlib.dumps(dict(CFBundleName='Xpotato Tunnel',CFBundleDisplayName='Xpotato Tunnel',CFBundleIdentifier='net.xpotato.tunnel',CFBundleExecutable='xpotato-tunnel-app',CFBundlePackageType='APPL',CFBundleShortVersionString=version,CFBundleVersion=version,CFBundleIconFile='icon.icns',LSMinimumSystemVersion='13.5',NSHighResolutionCapable=True)))
        contents=app/'Contents'
        (contents/'SHA256SUMS').write_text(''.join(f'{digest(p)}  {p.relative_to(contents).as_posix()}\n' for p in sorted(contents.rglob('*')) if p.is_file()),encoding='utf-8',newline='\n')
        spec=importlib.util.spec_from_file_location('package_review',ROOT/'scripts/package-review.py')
        package=importlib.util.module_from_spec(spec);spec.loader.exec_module(package);package.verify(contents)
        image=output/f'xpotato-tunnel-{version}-{platform}.dmg'
        # A fresh image only. No mount, Gatekeeper bypass, key or signing setup.
        subprocess.run(['hdiutil','create','-volname','Xpotato Tunnel','-srcfolder',str(image_root),'-format','UDZO',str(image)],check=True)
        return image
    if platform=='linux-amd64':
        root=output/'deb-root';app=root/'usr/lib/xpotato-tunnel'
        app.parent.mkdir(parents=True);shutil.copytree(payload,app)
        for name in ('xpotato-tunnel','xpotato-tunnel-app'):(app/name).chmod(0o755)
        control=root/'DEBIAN';control.mkdir()
        (control/'control').write_text(f'Package: xpotato-tunnel\nVersion: {version}\nArchitecture: amd64\nMaintainer: Xpotato.net\nSection: net\nPriority: optional\nDepends: libwebkit2gtk-4.1-0, libgtk-3-0t64\nDescription: Xpotato Tunnel desktop setup and maintenance\n',encoding='utf-8')
        desktop=root/'usr/share/applications';desktop.mkdir(parents=True)
        (desktop/'xpotato-tunnel.desktop').write_text('[Desktop Entry]\nType=Application\nName=Xpotato Tunnel\nComment=Set up and manage this device\nExec=/usr/lib/xpotato-tunnel/xpotato-tunnel-app\nIcon=xpotato-tunnel\nTerminal=false\nCategories=Network;Utility;\n',encoding='utf-8')
        icons=root/'usr/share/icons/hicolor/scalable/apps';icons.mkdir(parents=True)
        shutil.copy2(ROOT/'gui/src-tauri/icons/icon.svg',icons/'xpotato-tunnel.svg')
        archive=output/f'xpotato-tunnel-{version}-linux-amd64.deb'
        subprocess.run(['dpkg-deb','--root-owner-group','--build',str(root),str(archive)],check=True)
        return archive
    return None # Windows delegates to the existing pinned NSIS factory.

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ('content','gui','notices','output'):parser.add_argument('--'+name,type=Path,required=True)
    parser.add_argument('--platform',required=True)
    args=parser.parse_args()
    payload,info=assemble(args.content,args.gui,args.notices,args.output,args.platform)
    artifact=native(payload,info,args.output)
    if artifact:artifact.with_name(artifact.name+'.sha256').write_text(f'{digest(artifact)}  {artifact.name}\n',encoding='utf-8')
    print(artifact or payload)

if __name__=='__main__':main()
