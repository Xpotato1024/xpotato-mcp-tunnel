"""Publication input boundaries; tiny fixtures, no build/install/network."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[1];sys.path.insert(0,str(ROOT/'scripts'))
spec=importlib.util.spec_from_file_location('desktop_release',ROOT/'scripts/package-desktop-release.py')
release=importlib.util.module_from_spec(spec);spec.loader.exec_module(release)
HEAD='a'*40;CI='https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/runs/1'


class DesktopReleaseTests(unittest.TestCase):
    def fixture(self,root,source_overrides=None,filter_overrides=None):
        inputs=root/'inputs';inputs.mkdir();policy=release.profile('windows-linux')
        source=inputs/('source-windows-linux-'+HEAD);source.mkdir()
        info=dict(commit=HEAD,version='0.2.11',sourceProfile='windows-linux',rustDependenciesIncluded=True,goDependenciesIncluded=True,targets=policy['targets'],heldTargets=policy['heldTargets'],cargoLockSHA256='b'*64)
        filtering=dict(profile='windows-linux',targets=policy['targets'],heldTargets=policy['heldTargets'],lockSHA256='b'*64,implementationPackages=[dict(name='used')],resolutionOnlyStubs=[])
        info.update(source_overrides or {});filtering.update(filter_overrides or {})
        archive=source/'xpotato-tunnel-0.2.11-source.tar.gz'
        with tarfile.open(archive,'w:gz') as tar:
            for name,data in [('SOURCE-PROVENANCE.json',info),('SOURCE-FILTERING.json',filtering)]:
                raw=json.dumps(data).encode();member=tarfile.TarInfo(name);member.size=len(raw);tar.addfile(member,io.BytesIO(raw))
        files=[archive];results=[]
        for platform,target,ext in [('windows-amd64','x86_64-pc-windows-msvc','-setup.exe'),('linux-amd64','x86_64-unknown-linux-gnu','.deb')]:
            directory=inputs/('desktop-'+platform+'-'+HEAD);proof=directory/'source-rebuild/build/SOURCE-REBUILD-RESULT.json';proof.parent.mkdir(parents=True)
            result=dict(commit=HEAD,target=target,sourceProfile='windows-linux',cargoLockSHA256='b'*64,emptyCaches=True,cargoFrozen=True,goProxy='off',gui=dict(commit=HEAD,version='0.2.11',dirty=False,nativeFixture=False,coreSHA256='c'*64))
            proof.write_text(json.dumps(result));results.append(proof)
            package=directory/('installer' if platform.startswith('windows') else 'gui')/f'xpotato-tunnel-0.2.11-{platform}{ext}';package.parent.mkdir();package.write_bytes(b'fixture only');files.append(package)
        for f in files:f.with_name(f.name+'.sha256').write_text(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n')
        return inputs,files,results

    def test_eight_assets_and_no_private_paths_or_recursive_checksum(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);inputs,_,_=self.fixture(root);out=root/'release'
            data=release.assemble(inputs,out,HEAD,CI)
            self.assertEqual(len(list(out.iterdir())),8)
            self.assertEqual(data['commit'],HEAD)
            for line in (out/'SHA256SUMS.txt').read_text().splitlines():
                sha,name=line.split('  ');self.assertEqual(release.digest(out/name),sha);self.assertNotEqual(name,'SHA256SUMS.txt')
            text=(out/'release-provenance.json').read_text();self.assertNotIn(str(root),text);self.assertNotIn('native-fixture',text)
            with self.assertRaises(FileExistsError):release.assemble(inputs,out,HEAD,CI)

    def test_wrong_head_profile_production_proof_or_missing_offline_gate_fails(self):
        for field,value in [('commit','d'*40),('sourceProfile','core'),('emptyCaches',False),('cargoFrozen',False),('goProxy','https://proxy.example')]:
            with self.subTest(field=field),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);inputs,_,proofs=self.fixture(root);proof=proofs[0];data=json.loads(proof.read_text());data[field]=value;proof.write_text(json.dumps(data))
                with self.assertRaises(ValueError):release.assemble(inputs,root/'out',HEAD,CI)
                self.assertFalse((root/'out').exists())
        for field,value in [('dirty',True),('nativeFixture',True),('commit','e'*40)]:
            with self.subTest(gui=field),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);inputs,_,proofs=self.fixture(root);proof=proofs[0];data=json.loads(proof.read_text());data['gui'][field]=value;proof.write_text(json.dumps(data))
                with self.assertRaises(ValueError):release.assemble(inputs,root/'out',HEAD,CI)

    def test_corrupt_asset_or_extra_mac_candidate_is_never_copied(self):
        for scenario in ('digest','extra'):
            with self.subTest(scenario=scenario),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);inputs,files,_=self.fixture(root)
                if scenario=='digest':files[-1].write_bytes(b'changed')
                else:(inputs/('desktop-darwin-arm64-'+HEAD)).mkdir()
                with self.assertRaises(ValueError):release.assemble(inputs,root/'out',HEAD,CI)
                self.assertFalse((root/'out').exists())

    def test_core_only_source_or_binding_implementation_report_is_rejected(self):
        for source,filtering in [({'sourceProfile':'core'},{}),({'rustDependenciesIncluded':False},{}),({}, {'implementationPackages':[dict(name='objc2-app-kit')]})]:
            with self.subTest(source=source,filtering=filtering),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);inputs,_,_=self.fixture(root,source,filtering)
                with self.assertRaises(ValueError):release.assemble(inputs,root/'out',HEAD,CI)
                self.assertFalse((root/'out').exists())


if __name__=='__main__':unittest.main()
