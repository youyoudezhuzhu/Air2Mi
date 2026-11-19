import base64,hashlib,importlib.util,io,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
spec=importlib.util.spec_from_file_location('online_client',Path(__file__).resolve().parents[2]/'android/app/src/main/assets/air2mi_update_client.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class Response(io.BytesIO):
 status=200
class OnlineUpdatesTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
  self.key=Ed25519PrivateKey.generate();self.public=self.key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
  self.client=m.Client(self.root/'updates',base64.b64encode(self.public).decode(),'org.air2mi.app',1)
  self.data=b'APK regression bytes; not used as an installable Android package'
  self.target=dict(app_version='1.1.1',platform='android',architecture='arm64-v8a',kind='apk',api_version='v1',application_id='org.air2mi.app',version_code=2,size=len(self.data),sha256=hashlib.sha256(self.data).hexdigest(),signer_sha256='a'*64,source_commit='b'*40,package_version='0.3.11-stable.1-hardened',kernel_version='0.3.11',url='https://youyoudezhuzhu.github.io/Air2Mi/dl/app.apk',url_alt='https://github.com/youyoudezhuzhu/Air2Mi/releases/download/v1/app.apk')
  self.catalog=dict(component="engine",schema_version=1,app_id='air2mi',channel='stable',published=True,sequence=7,targets=[self.target])
 def signed(self,catalog=None):
  encode=lambda value:base64.urlsafe_b64encode(value).decode().rstrip('=')
  raw=json.dumps(catalog or self.catalog,ensure_ascii=False,indent=2).encode()
  return json.dumps(dict(format='air2mi-update-v1',key_id='air2mi-update-v1',payload=encode(raw),signature=encode(self.key.sign(raw)))).encode()
 def check(self):
  with patch.object(self.client,'open',return_value=Response(self.signed())):return json.loads(self.client.check())
 def test_component_update_is_independent_of_apk_version(self):
  self.target.update(kind='engine_components')
  for name in ('application_id','app_version','version_code','signer_sha256'): del self.target[name]
  status=self.check()
  self.assertTrue(status['available'])
  self.client.begin(status['release_id'])
  with patch.object(self.client,'open',return_value=Response(self.data)):
   output=Path(self.client.download())
  self.assertEqual(output.suffix,'.zip')
  self.assertEqual(output.read_bytes(),self.data)
  self.client.imported(True)
  self.assertEqual(json.loads(self.client.snapshot())['phase'],'installed')
 def test_signature_raw_bytes_and_tamper_rejection(self):
  self.assertEqual(m.verify(self.signed(),self.public)['sequence'],7)
  envelope=json.loads(self.signed());envelope['payload']=base64.urlsafe_b64encode(b'{}').decode().rstrip('=')
  with self.assertRaises(m.UpdateError):m.verify(json.dumps(envelope).encode(),self.public)
  with self.assertRaises(m.UpdateError):m.verify(self.signed(),bytes(32))
 def test_wrong_package_and_replayed_sequence_fail_closed(self):
  self.target['application_id']='another.app';self.assertEqual(self.check()['phase'],'error')
  self.target['application_id']='org.air2mi.app';(self.client.folder/'accepted-sequence.json').write_text('{"sequence":8}')
  self.assertEqual(self.check()['phase'],'error')
 def test_unsigned_display_status_does_not_create_task(self):
  with self.assertRaises(m.UpdateError):self.client.begin('https://attacker.invalid/file')
  self.catalog['published']=False;self.assertEqual(self.check()['phase'],'unpublished')
  with self.assertRaises(m.UpdateError):self.client.begin('7:'+self.target['sha256'])
 def test_primary_corruption_uses_fallback_and_preserves_user_data(self):
  marker=self.root/'engine-data/identity';marker.parent.mkdir();marker.write_bytes(b'keep')
  status=self.check();self.client.begin(status['release_id'])
  def source(url):return Response(b'bad' if url==self.target['url'] else self.data)
  with patch.object(self.client,'open',side_effect=source):path=Path(self.client.download())
  self.assertEqual(path.read_bytes(),self.data);self.assertEqual(marker.read_bytes(),b'keep')
  self.assertEqual(json.loads(self.client.snapshot())['phase'],'install_ready')
  self.assertEqual(json.loads((self.client.folder/'accepted-sequence.json').read_text())['sequence'],7)
 def test_failed_download_does_not_replace_previous_apk(self):
  old=self.client.folder/'engine-update.apk';old.write_bytes(b'previous')
  status=self.check();self.client.begin(status['release_id'])
  with patch.object(self.client,'open',side_effect=lambda _:Response(b'corrupt')),self.assertRaises(m.UpdateError):self.client.download()
  self.assertEqual(old.read_bytes(),b'previous');self.assertFalse((self.client.folder/'engine-update.partial').exists())
 def test_failed_foreground_start_unlocks_future_checks(self):
  status=self.check();self.client.begin(status['release_id']);self.client.failed('service denied')
  self.assertFalse(self.client.working);self.assertEqual(self.check()['phase'],'available')
 def test_download_never_accepts_private_or_plain_http_sources(self):
  for url in ('http://youyoudezhuzhu.github.io/Air2Mi/a','https://127.0.0.1/a','https://user:pass@github.com/youyoudezhuzhu/Air2Mi/releases/download/v1/a'):
   with self.subTest(url=url),self.assertRaises(m.UpdateError):self.client.allowed(url)

 def test_app_and_engine_catalogs_cannot_be_interchanged(self):
  app=m.Client(self.root/'app-updates',base64.b64encode(self.public).decode(),'org.air2mi.app',1,component='app',app_version='1.0.0')
  self.assertNotEqual(app.source,self.client.source)
  with patch.object(app,'open',return_value=Response(self.signed())):self.assertEqual(json.loads(app.check())['phase'],'error')
  self.catalog['component']='app';self.target['app_version']='1.1.0'
  with patch.object(app,'open',return_value=Response(self.signed())):state=json.loads(app.check())
  self.assertTrue(state['available']);self.assertEqual(state['current_version'],'1.0.0')
  self.assertEqual(json.loads(self.client.snapshot())['phase'],'idle')
 def test_engine_comparison_uses_engine_version_not_only_apk_code(self):
  with patch.object(self.client,'open',return_value=Response(self.signed())):
   state=json.loads(self.client.check(json.dumps({'package_version':self.target['package_version']})))
  self.assertFalse(state['available'])
  self.client.version=2
  state=self.check();self.assertEqual(state['phase'],'requires_new_package');self.assertFalse(state['available'])

 def test_engine_upgrade_must_not_downgrade_app(self):
  self.client.app_version='1.2.0'
  self.assertEqual(self.check()['phase'],'error')
