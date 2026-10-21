import hashlib
import importlib.util
import json
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
def load(name,file):
    spec=importlib.util.spec_from_file_location(name,ROOT/'scripts'/file)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    return module
gui=load('gui_package','package-gui.py')
core=load('core_package','package-review.py')
notices=load('gui_notices','gui-notices.py')

class GUIPackageTests(unittest.TestCase):
    def test_native_layout_and_exact_sidecar_provenance(self):
        for platform in ('windows-amd64','linux-amd64','darwin-amd64','darwin-arm64'):
            with self.subTest(platform=platform),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);binary=root/'core';binary.write_bytes(b'fixed core')
                _,content=core.assemble(binary,root/'core-package','0.2.11','a'*40,platform,False,'native fixture')
                app=root/'app';app.write_bytes(b'fixed GUI');notice=root/'notices';notice.write_text('license fixture\n'*100)
                info=dict(version='0.2.11',commit='a'*40,dirty=False,nativeFixture=False,coreSHA256=hashlib.sha256(binary.read_bytes()).hexdigest())
                with patch.object(gui.subprocess,'check_output',return_value=json.dumps(info)):
                    payload,metadata=gui.assemble(content,app,notice,root/'gui',platform)
                core.verify(payload)
                with patch.object(gui.subprocess,'run') as command:
                    artifact=gui.native(payload,metadata,root/'gui')
                if platform.startswith('darwin'):
                    bundle=root/'gui/dmg-root/Xpotato Tunnel.app/Contents'
                    plist=plistlib.loads((bundle/'Info.plist').read_bytes())
                    core.verify(bundle)
                    self.assertEqual(plist['CFBundleExecutable'],'xpotato-tunnel-app')
                    self.assertEqual(plist['LSMinimumSystemVersion'],'13.5')
                    self.assertEqual((bundle/'MacOS/xpotato-tunnel').read_bytes(),b'fixed core')
                    self.assertTrue((bundle/'Resources/licenses/GUI-THIRD-PARTY-LICENSES.txt').is_file())
                    self.assertEqual(command.call_args.args[0][:2],['hdiutil','create'])
                    arguments=command.call_args.args[0]
                    image_root=Path(arguments[arguments.index('-srcfolder')+1])
                    self.assertEqual([p.name for p in image_root.iterdir()],['Xpotato Tunnel.app'])
                    self.assertEqual(artifact.suffix,'.dmg')
                elif platform=='linux-amd64':
                    deb=root/'gui/deb-root'
                    desktop=(deb/'usr/share/applications/xpotato-tunnel.desktop').read_text()
                    self.assertIn('Exec=/usr/lib/xpotato-tunnel/xpotato-tunnel-app',desktop)
                    self.assertEqual(set(p.name for p in (deb/'DEBIAN').iterdir()),{'control'})
                    self.assertEqual(command.call_args.args[0][:2],['dpkg-deb','--root-owner-group'])
                else:self.assertIsNone(artifact)
                info['nativeFixture']=True
                with patch.object(gui.subprocess,'check_output',return_value=json.dumps(info)),self.assertRaisesRegex(ValueError,'non-distributable'):
                    gui.assemble(content,app,notice,root/'fixture-rejected',platform)
                self.assertFalse((root/'fixture-rejected').exists())
                del info['nativeFixture']
                with patch.object(gui.subprocess,'check_output',return_value=json.dumps(info)),self.assertRaisesRegex(ValueError,'production marker'):
                    gui.assemble(content,app,notice,root/'unmarked-rejected',platform)
                self.assertFalse((root/'unmarked-rejected').exists())
                info['nativeFixture']=False
                info['coreSHA256']='0'*64
                with patch.object(gui.subprocess,'check_output',return_value=json.dumps(info)),self.assertRaisesRegex(ValueError,'provenance'):
                    gui.assemble(content,app,notice,root/'invalid',platform)
                self.assertFalse((root/'invalid').exists())
                nested=content/'docs/SHA256SUMS';nested.write_text('untracked nested manifest')
                with self.assertRaisesRegex(ValueError,'inventory'):core.verify(content)
                nested.unlink()
                (content/'unexpected.conf').write_text('must not be distributed')
                with patch.object(gui.subprocess,'check_output',return_value=json.dumps(info)),self.assertRaisesRegex(ValueError,'inventory'):
                    gui.assemble(content,app,notice,root/'injected',platform)

    def test_override_requires_exact_upstream_commit(self):
        with tempfile.TemporaryDirectory() as temp:
            directory=Path(temp)
            record=json.loads((ROOT/'gui/licenses/overrides.json').read_text())['selectors@0.38.0']
            package=dict(name='selectors',version='0.38.0',license='MPL-2.0')
            vcs=directory/'.cargo_vcs_info.json'
            vcs.write_text(json.dumps({'git':{'sha1':record['commit']}}))
            self.assertIn('Mozilla Public License',notices.override_text(package,directory))
            vcs.write_text(json.dumps({'git':{'sha1':'0'*40}}))
            with self.assertRaisesRegex(ValueError,'source differs'):notices.override_text(package,directory)

    def test_selected_mit_keeps_upstream_declaration_and_standard_text(self):
        record=json.loads((ROOT/'gui/licenses/overrides.json').read_text())['objc2-web-kit@0.3.2']
        with tempfile.TemporaryDirectory() as temp:
            directory=Path(temp)
            (directory/'.cargo_vcs_info.json').write_text(json.dumps({'git':{'sha1':record['commit']}}))
            text=notices.override_text(dict(name='objc2-web-kit',version='0.3.2',license=record['license']),directory)
            self.assertIn('Selected license: MIT',text)
            self.assertIn('derived from Apple SDKs',text)
            self.assertIn('Permission is hereby granted',text)
            self.assertIn('<copyright holders>',text) # Preserve template; never invent attribution.

if __name__=='__main__':unittest.main()
