"""Explicit local-test exception; the default APK audit still rejects this APK."""
import argparse,hashlib,importlib.util,io,json,zipfile
from pathlib import Path
ASSET='assets/private/air2mi-engine-components.zip'
def check(apk,expected):
 if len(expected)!=64 or any(c not in '0123456789abcdef' for c in expected):raise ValueError('pinned SHA256 required')
 spec=importlib.util.spec_from_file_location('app_audit',Path(__file__).with_name('check_android_apk.py'))
 audit=importlib.util.module_from_spec(spec);spec.loader.exec_module(audit)
 with zipfile.ZipFile(apk) as source:
  data=source.read(ASSET)
  if hashlib.sha256(data).hexdigest()!=expected:raise ValueError('bundled Engine mismatch')
  rest=io.BytesIO()
  with zipfile.ZipFile(rest,'w') as out:
   for name in source.namelist():
    if name!=ASSET:out.writestr(name,source.read(name))
  audit.audit(rest.getvalue())
  with zipfile.ZipFile(io.BytesIO(data)) as engine:
   manifest=json.loads(engine.read('assets/engine-build.json'))
   assert manifest['platform']=='android' and manifest['architecture']=='arm64-v8a'
   assert manifest['api_version']=='v1' and manifest['hardening']['native_entitlement_gate']
   assert hashlib.sha256(engine.read('assets/engine-native.zip')).hexdigest()==manifest['files']['engine-native.zip']
 return dict(bundled_test_engine=True,engine_sha256=expected,engine_version=manifest['package_version'],engine_source_revision=manifest['source_revision'],rest_of_app_engine_free=True)
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('apk');p.add_argument('--sha256',required=True);a=p.parse_args();print(json.dumps(check(a.apk,a.sha256)))
