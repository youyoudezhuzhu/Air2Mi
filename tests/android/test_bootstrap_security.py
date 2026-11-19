import hashlib,importlib.util,json,tempfile,types,unittest,zipfile,sys
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('loader',ROOT/'android/app/src/main/assets/air2mi_android_bootstrap.py')
loader=importlib.util.module_from_spec(spec);spec.loader.exec_module(loader)
class BootstrapSecurityTests(unittest.TestCase):
 def assets(self,base,files):
  archive=base/'engine-native.zip'
  with zipfile.ZipFile(archive,'w') as z:
   for name,data in files.items():z.writestr(name,data)
  (base/'engine-build.json').write_text(json.dumps({'files':{archive.name:hashlib.sha256(archive.read_bytes()).hexdigest()}}))
 def test_reuse_verifies_modified_missing_and_extra_modules(self):
  for change in ('modify','delete','extra'):
   with self.subTest(change=change),tempfile.TemporaryDirectory() as tmp:
    base=Path(tmp);self.assets(base,{'pkg/module.so':b'valid'});root,_=loader.unpack(base)
    if change=='modify':(root/'pkg/module.so').write_bytes(b'other')
    elif change=='delete':(root/'pkg/module.so').unlink()
    else:(root/'shadow.py').write_text('pass')
    with self.assertRaises(ValueError):loader.unpack(base)
 def test_valid_reuse_never_overwrites_mapped_files(self):
  with tempfile.TemporaryDirectory() as tmp:
   base=Path(tmp);self.assets(base,{'module.so':b'valid'});root,_=loader.unpack(base);before=(root/'module.so').stat()
   self.assertEqual(loader.unpack(base)[0],root);after=(root/'module.so').stat()
   self.assertEqual((before.st_ino,before.st_mtime_ns),(after.st_ino,after.st_mtime_ns))
 def test_unsafe_paths_and_root_links_rejected(self):
  with tempfile.TemporaryDirectory() as tmp:
   base=Path(tmp)
   for name in ('../escape','a//b','a/./b','/absolute','a\\b'):
    self.assets(base,{name:b'x'})
    with self.assertRaises(ValueError):loader.unpack(base)
   self.assets(base,{'module.so':b'valid'});root,_=loader.unpack(base);saved=base/'saved';root.rename(saved);root.symlink_to(saved,target_is_directory=True)
   with self.assertRaises(ValueError):loader.unpack(base)
 def test_anchor_cannot_be_loaded_from_another_build(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp);(root/'_air2mi_integrity.so').write_bytes(b'test')
   with patch.dict(sys.modules,{'_air2mi_integrity':types.SimpleNamespace(__file__=str(root/'old.so'))}):
    with self.assertRaisesRegex(ValueError,'restart'):loader.load_anchor(root)
