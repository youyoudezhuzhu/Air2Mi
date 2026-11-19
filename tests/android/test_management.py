import copy,importlib.util,io,json,sys,tempfile,unittest,wave
from pathlib import Path
from unittest.mock import patch
from urllib import error
root=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('app_host',root/'android/app/src/main/assets/air2mi_app_host.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class Owner:
 def __init__(self):self.data={'inputs':[],'routes':[],'speakers':[{'speaker_id':'a','display_name':'Kitchen','ip':'192.0.2.1','enabled':True}]};self.calls=[]
 def callEngine(self,path,method,body):
  self.calls.append((path,method,json.loads(body) if body else None))
  if path.startswith('/v1/license'):return json.dumps({'status':429,'body':{'error':'RATE_LIMITED','retry_after':37}})
  if path=='/v1/state':return json.dumps({'status':200,'body':{'river':{'producer':None},'instances':{},'speakers':[]}})
  kind=path.split('/')[2]
  if method=='GET':v={kind:self.data[kind]}
  else:
   v=json.loads(body or '{}')
   if method=='POST':self.data[kind].append(copy.deepcopy(v))
  return json.dumps({'status':200,'body':v})
class Response(io.BytesIO):
 def __init__(self,value,status=200,headers=None):super().__init__(json.dumps(value).encode());self.status=status;self.headers=headers or {}
class ManagementTests(unittest.TestCase):
 def setUp(self):self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.owner=Owner();self.host=m.Manager(self.owner,self.temp.name,self.temp.name)
 def test_receiver_only_asks_name_target_enabled(self):
  out=json.loads(self.host.request('/api/engine/v1/inputs','POST',json.dumps({'display_name':'Kitchen AirPlay','enabled':False,'options':{'speaker_id':'a'}})))
  self.assertEqual(out['status'],200);self.assertTrue(out['body']['input_id'].startswith('ap-'));self.assertGreaterEqual(out['body']['port'],7100)
 def test_one_to_one_cannot_be_bypassed_through_host(self):
  self.owner.data['inputs']=[{'input_id':'first','display_name':'First','options':{'speaker_id':'a'}}]
  out=json.loads(self.host.request('/api/engine/v1/inputs','POST',json.dumps({'display_name':'Second','options':{'speaker_id':'a'}})))
  self.assertEqual(out['status'],409);self.assertEqual(len(self.owner.data['inputs']),1)
 def test_browser_cannot_supply_ports_paths_or_sessions(self):
  for path,body in [('/api/engine/v1/inputs',{'port':7100}),('/api/engine/v1/sessions',{'file':'/etc/passwd'}),('/api/speakers/a/test-tone',{'volume_percent':25,'file':'/etc/passwd'})]:
   with self.subTest(path=path):self.assertGreaterEqual(json.loads(self.host.request(path,'POST',json.dumps(body)))['status'],400)
  self.assertFalse(any(p=='/v1/sessions' for p,_,_ in self.owner.calls))
 def test_license_retry_metadata_is_preserved(self):
  out=json.loads(self.host.request('/api/engine/v1/license/restore','POST','{}'));self.assertEqual((out['status'],out['body']['retry_after']),(429,37))
 def test_raw_device_code_and_secrets_never_cross_ui(self):
  real='AMD1-'+'-'.join(['1234']*8);out=m.public({'device_id':real,'error':'设备码'+real,'token':'private','signature':'private','nested':{'credential':{}}})
  text=json.dumps(out);self.assertNotIn('AMD1-',text);self.assertNotIn('private',text);self.assertRegex(out['device_id'],r'^AVD1-(?:[A-F0-9]{4}-){7}[A-F0-9]{4}$')
  self.assertEqual(json.loads(m.public_json(json.dumps({'device_id':real})))['device_id'],out['device_id'])
 def test_busy_inputs_prevent_test_session(self):
  for state in [{'river':{'producer':'a'},'instances':{}},{'river':{},'instances':{'a':{'selected':True}}},{'river':{},'instances':{},'speakers':[{'connected':True}]}]:
   with self.assertRaises(m.Failure):m.idle(state)
 def test_test_tone_volume_boundaries_and_single_target(self):
  for volume in [0,101,True,'25']:
   with self.assertRaises(m.Failure):self.host.tone.start('a',{'volume_percent':volume})
  with self.assertRaises(m.Failure):self.host.tone.start('unknown',{'volume_percent':25})
 def test_configuration_is_preserved_on_stopped_reads(self):
  path=Path(self.temp.name)/'registry.json';path.write_text(json.dumps(self.owner.data));before=path.read_bytes()
  self.assertEqual(self.host.configuration(),self.owner.data);self.assertEqual(path.read_bytes(),before)
 def test_logs_are_filtered_and_redacted(self):
  real='AMD1-'+'-'.join(['1234']*8);(Path(self.temp.name)/'app.log').write_text('INFO hello\nERROR '+real+' Bearer SECRET\n')
  result=self.host.logs({'level':['error']})['text'];self.assertIn('ERROR',result);self.assertNotIn('AMD1-',result);self.assertNotIn('SECRET',result);self.assertNotIn('hello',result)
 def test_fixed_asset_is_bounded_pcm_and_not_clipped(self):
  path=Path(self.temp.name)/'tone.wav';m.write_tone(path)
  with wave.open(str(path)) as audio:self.assertEqual((audio.getframerate(),audio.getnchannels(),audio.getsampwidth()),(44100,2,2));self.assertGreater(audio.getnframes()/44100,300)
class OfferTests(unittest.TestCase):
 def value(self):return dict(schema_version=1,app_id='air2mi',product_id='plus',enabled=True,purchasable=True,currency='CNY',price_minor=1990,button_text='购买',purchase_url='https://example.com')
 def test_cache_etag_and_304(self):
  clock=[0];calls=[]
  def open(req,timeout):calls.append(req);return Response(self.value(),headers={'ETag':'v1'}) if len(calls)==1 else Response({},304)
  offer=m.Offer(open,lambda:clock[0]);offer.get();offer.get();self.assertEqual(len(calls),1);clock[0]=61;self.assertEqual(offer.get()['price_minor'],1990);self.assertEqual(calls[1].headers['If-none-match'],'v1')
 def test_invalid_and_rate_limited_offers_fail_closed(self):
  for response in [Response({'ok':True}),Response({},429,{'Retry-After':'23'})]:
   with self.assertRaises(m.Failure):m.Offer(lambda *a,**k:response).get()

class DiscoveryIdentityTests(unittest.TestCase):
 def test_uuid_variant_preserves_referenced_device(self):
  old={'speaker_id':'saved','display_name':'My kitchen','options':{'discovery_id':'12345678-1234-1234-1234-123456789abc'}}
  found={'speaker_id':'new','options':{'discovery_id':'12345678-1234-1234-1234-123456789abd'}}
  selected,_=m.discovery_match(found,[old],{'saved':1});self.assertIs(selected,old)
 def test_ambiguous_duplicate_does_not_delete_associations(self):
  key='12345678-1234-1234-1234-123456789abc';rows=[{'speaker_id':x,'options':{'discovery_id':key}} for x in ('a','b')]
  with self.assertRaises(m.Failure):m.discovery_match({'speaker_id':'found','options':{'discovery_id':key}},rows,{'a':1,'b':1})
 def test_same_name_or_model_does_not_define_device_identity(self):
  selected,_=m.discovery_match({'speaker_id':'new','options':{'discovery_id':'bbbbbbbb-1234-1234-1234-123456789abc'}},[{'speaker_id':'old','options':{'discovery_id':'aaaaaaaa-1234-1234-1234-123456789abc'}}],{'old':1});self.assertIsNone(selected)

class ToneLifecycleTests(unittest.TestCase):
 def test_only_controlled_asset_and_target_are_used_and_daily_host_is_restored(self):
  import dataclasses,types,uuid
  @dataclasses.dataclass
  class Config:daily_mode:bool=True
  class Host:
   def __init__(self):self.running=False;self.config=None
   def initialize(self,config,*args,**kwargs):self.config=config;created.append(self)
   def start(self):self.running=True
   def get_api_endpoint(self):return {'url':'http://127.0.0.1:9','token':'test-token'}
   def status(self):return {'running':self.running,'state':{'speakers':[{'speaker_id':'a','first_audio_pcm':True}]}}
   def shutdown(self):self.running=False
  for failure in [False,True]:
   with self.subTest(session_failure=failure),tempfile.TemporaryDirectory() as tmp:
    owner=Owner();owner.getBindAddress=lambda:'192.0.2.2';lifecycle=[];owner.suspendForTone=lambda:lifecycle.append('suspend');owner.resumeAfterTone=lambda:lifecycle.append('resume')
    manager=m.Manager(owner,str(Path(tmp)/'engine-data'),tmp);manager.tone.state={'id':'tone-'+uuid.uuid4().hex,'speaker_id':'a'};created=[];calls=[]
    def open(req,timeout):
     payload=json.loads(req.data);calls.append((req.full_url,payload))
     if failure and req.full_url.endswith('/v1/sessions'):raise RuntimeError('session failed')
     if req.full_url.endswith('/v1/sessions'):manager.tone.cancel.set()
     return Response({})
    modules={'a2m_kernel':types.ModuleType('a2m_kernel'),'a2m_kernel.host':types.SimpleNamespace(EngineHost=Host),'engine_runtime':types.ModuleType('engine_runtime'),'engine_runtime.configuration':types.SimpleNamespace(configure_host=lambda *a:Config())}
    with patch.dict(sys.modules,modules),patch.object(m.request,'urlopen',open):manager.tone.run(owner.data['speakers'][0],25)
    session=next(body for url,body in calls if url.endswith('/v1/sessions'));self.assertEqual(session['speaker_ids'],['a']);self.assertEqual(session['kind'],'file');self.assertTrue(Path(session['file']).is_relative_to(Path(tmp)))
    self.assertEqual(lifecycle,['suspend','resume']);self.assertFalse(created[0].running);self.assertFalse(created[0].config.daily_mode);self.assertFalse(Path(session['file']).exists());self.assertEqual(manager.tone.state['phase'],'failed' if failure else 'stopped')
 def test_incomplete_native_cleanup_blocks_new_playback(self):
  with tempfile.TemporaryDirectory() as tmp:
   manager=m.Manager(Owner(),tmp,tmp);manager.tone.cleanup_failure=True
   with self.assertRaises(m.Failure):manager.tone.ensure_clean()
   self.assertTrue(manager.tone.busy())

if __name__=='__main__':unittest.main()
