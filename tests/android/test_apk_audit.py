import importlib.util,io,unittest,zipfile
from pathlib import Path
spec=importlib.util.spec_from_file_location('audit',Path(__file__).resolve().parents[2]/'tools/check_android_apk.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
def archive(files):
 target=io.BytesIO()
 with zipfile.ZipFile(target,'w') as out:
  for name,data in files.items():out.writestr(name,data)
 return target.getvalue()
class AuditTests(unittest.TestCase):
 def test_generic_python_runtime_is_allowed(self):module.audit(archive({'assets/python.imy':archive({'json/__init__.pyc':b'host runtime'})}))
 def test_nested_engine_archive_is_rejected(self):
  for name in ['engine-native.zip','engine_runtime/licensing.so','a2m_kernel/server.pyc','_air2mi_integrity.so']:
   with self.subTest(name=name),self.assertRaises(AssertionError):module.audit(archive({'assets/python.imy':archive({name:b'private engine'})}))
 def test_private_loader_inside_runtime_is_rejected(self):
  nested=archive({'air2mi_android_bootstrap.pyc':b'loader'})
  with self.assertRaises(AssertionError):
   module.audit(archive({'assets/chaquopy/app.imy':nested}))
  module.audit(archive({'assets/chaquopy/app.imy':archive({}), 'assets/air2mi_android_bootstrap.py':b'app-owned'}))
 def test_private_build_path_in_nested_native_library_is_rejected(self):
  requirements=archive({'bindings/test.so':b'ELF /vol9/@appdata/example/build'})
  with self.assertRaisesRegex(AssertionError,'Private build path'):
   module.audit(archive({'assets/chaquopy/requirements-common.imy':requirements}))
