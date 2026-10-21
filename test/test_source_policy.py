"""Source-distribution invariants; no app, registry, compiler or OS changes."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'scripts'))
import source_policy as policy

def load(name,file):
    spec=importlib.util.spec_from_file_location(name,ROOT/'scripts'/file)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module

rebuild=load('rebuild_source','rebuild-source.py')
source=load('source_package_policy_test','package-source.py')

class SourcePolicyTests(unittest.TestCase):
    def test_catalog_orphan_is_not_an_active_dependency(self):
        data=dict(packages=[dict(id=x,name=x) for x in ('root','used','objc2')],
            resolve=dict(root='root',nodes=[dict(id='root',deps=[dict(pkg='used')]),dict(id='used',deps=[])]))
        self.assertEqual(set(policy.selected(data)),{'root','used'})
        data['resolve']['nodes'][0]['deps'].append(dict(pkg='missing'))
        with self.assertRaisesRegex(ValueError,'incomplete'):policy.selected(data)

    def test_apple_binding_is_never_an_active_supported_target_dependency(self):
        p=policy.profile('windows-linux')
        data=dict(packages=[dict(id='root',name='app'),dict(id='apple',name='objc2-app-kit')],
            resolve=dict(root='root',nodes=[dict(id='root',deps=[dict(pkg='apple')]),dict(id='apple',deps=[])]))
        with patch.object(policy,'metadata',return_value=data),self.assertRaisesRegex(ValueError,'excluded binding is reachable'):
            policy.graph(ROOT,p)

    def fixture(self,root):
        p=policy.profile('windows-linux')
        base=root/'gui/src-tauri';base.mkdir(parents=True)
        locked=[]
        for name in ('used','objc2-app-kit'):
            locked.append(f'[[package]]\nname = "{name}"\nversion = "1.0.0"\nsource = "registry+https://github.com/rust-lang/crates.io-index"\nchecksum = "'+('a'*64)+'"\n')
            directory=root/'gui/vendor'/(name+'-1.0.0');(directory/'src').mkdir(parents=True)
            (directory/'Cargo.toml').write_text(f'[package]\nname = "{name}"\nversion = "1.0.0"\n',encoding='utf-8')
            (directory/'src/lib.rs').write_bytes(b'pub fn used() {}\n' if name=='used' else b'')
            self.update_checksums(directory)
        (base/'Cargo.lock').write_text('version = 4\n'+''.join(locked),encoding='utf-8')
        active=dict(root=dict(name='app',version='1.0.0',source=None),used=dict(name='used',version='1.0.0',source='registry'))
        return p,active,root/'gui/vendor/objc2-app-kit-1.0.0'

    def update_checksums(self,directory):
        data=dict(package='a'*64,files={p.relative_to(directory).as_posix():policy.digest(p) for p in directory.rglob('*') if p.is_file() and p.name!='.cargo-checksum.json'})
        (directory/'.cargo-checksum.json').write_text(json.dumps(data),encoding='utf-8')

    def test_stub_is_resolution_only_and_explicitly_refuses_compilation(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);p,active,stub=self.fixture(root);policy.harden_stubs(root)
            self.assertIn(b'compile_error!',(stub/'src/lib.rs').read_bytes())
            with patch.object(policy,'graph',return_value=active):report=policy.audit_vendor(root,p)
            self.assertEqual([x['name'] for x in report['implementationPackages']],['used'])
            self.assertEqual([x['name'] for x in report['resolutionOnlyStubs']],['objc2-app-kit'])
            self.assertEqual(set(policy.files(stub)),policy.STUB_FILES)

    def test_stub_cannot_be_used_by_a_supported_target(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);p,active,stub=self.fixture(root);policy.harden_stubs(root)
            active['apple']=dict(name='objc2-app-kit',version='1.0.0',source='registry')
            with patch.object(policy,'graph',return_value=active),self.assertRaisesRegex(ValueError,'active dependency replaced'):
                policy.audit_vendor(root,p)

    def test_remaining_binding_code_extra_files_or_altered_pin_fail_closed(self):
        for scenario in ('implementation','extra','pin','missing'):
            with self.subTest(scenario=scenario),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);p,active,stub=self.fixture(root);policy.harden_stubs(root)
                if scenario=='implementation':(stub/'src/lib.rs').write_bytes(b'// SDK implementation');self.update_checksums(stub)
                if scenario=='extra':(stub/'SDK.h').write_bytes(b'SDK bytes');self.update_checksums(stub)
                if scenario=='pin':
                    checks=json.loads((stub/'.cargo-checksum.json').read_text());checks['package']='b'*64
                    (stub/'.cargo-checksum.json').write_text(json.dumps(checks))
                if scenario=='missing':(stub/'src/lib.rs').unlink()
                with patch.object(policy,'graph',return_value=active),self.assertRaises(ValueError):policy.audit_vendor(root,p)

    def test_unmodified_source_inventory_and_hash_are_required(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);content=root/'source';content.mkdir();(content/'code.rs').write_bytes(b'fn main() {}')
            source.pack(content,root/'fixture.tar.gz',1792392030)
            rebuild.verify_source(content)
            (content/'unexpected.rs').write_bytes(b'extra')
            with self.assertRaisesRegex(ValueError,'inventory'):rebuild.verify_source(content)
            (content/'unexpected.rs').unlink();(content/'code.rs').write_bytes(b'changed')
            with self.assertRaisesRegex(ValueError,'source changed'):rebuild.verify_source(content)

    def test_target_policy_keeps_common_source_and_holds_mac(self):
        p=policy.profile('windows-linux')
        self.assertEqual(set(p['targets']),{'x86_64-pc-windows-msvc','x86_64-unknown-linux-gnu'})
        self.assertEqual(set(p['heldTargets']),{'x86_64-apple-darwin','aarch64-apple-darwin'})
        for name in ('objc2','objc2-web-kit','block2','dispatch2','core-foundation-sys','swift-rs'):
            self.assertTrue(policy.excluded(name,p))
        self.assertFalse(policy.excluded('windows',p))

    @unittest.skipUnless(os.name=='nt','explicit Windows file API path form')
    def test_long_paths_are_included_without_changing_os_policy(self):
        with tempfile.TemporaryDirectory() as temp:
            base=Path(temp);deep=base
            while len(str(deep))<280:deep=deep/'source-path-segment'
            policy.long_path(deep).mkdir(parents=True)
            file=policy.long_path(deep/'LICENSE');file.write_bytes(b'full source license')
            try:
                found=policy.files(base)
                self.assertEqual(len(found),1)
                self.assertEqual(policy.digest(next(iter(found.values()))),hashlib.sha256(b'full source license').hexdigest())
            finally:
                file.unlink()
                while deep!=base:policy.long_path(deep).rmdir();deep=deep.parent

if __name__=='__main__':unittest.main()
