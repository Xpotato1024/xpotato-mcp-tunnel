import hashlib
import importlib.util
from pathlib import Path
import tarfile
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('source_package',Path(__file__).resolve().parents[1]/'scripts/package-source.py')
source=importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)

class SourcePackageTests(unittest.TestCase):
    def test_reproducible_and_exclusive_with_all_files_checksummed(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp); content=root/'content'; content.mkdir()
            (content/'LICENSE').write_bytes(b'legal text')
            (content/'code.go').write_bytes(b'package main\n')
            a=root/'a.tar.gz'; b=root/'b.tar.gz'
            source.pack(content,a,1792392030)
            source.pack(content,b,1792392030)
            self.assertEqual(a.read_bytes(),b.read_bytes())
            with self.assertRaises(FileExistsError): source.pack(content,a,1792392030)
            with tarfile.open(b) as archive:
                self.assertEqual(set(archive.getnames()),{'LICENSE','code.go','SOURCE-SHA256SUMS'})
                for line in archive.extractfile('SOURCE-SHA256SUMS').read().decode().splitlines():
                    digest,name=line.split('  ',1)
                    self.assertEqual(digest,hashlib.sha256(archive.extractfile(name).read()).hexdigest())
                self.assertTrue(all(m.uid==0 and m.gid==0 and m.mtime==1792392030 for m in archive))

if __name__=='__main__': unittest.main()
