#!/usr/bin/env python3
"""Real pinned Engine, isolated empty Registry; never advertises receivers or plays audio."""
import argparse,json,os,subprocess,tempfile,time,urllib.request
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--bundle',type=Path,required=True);p.add_argument('--host',type=Path,default=Path('build/air2mi-host'));p.add_argument('--keep',action='store_true');a=p.parse_args()
root=Path(__file__).resolve().parents[1];folder=Path(tempfile.mkdtemp(prefix='air2mi-integration-'));key=os.urandom(24).hex();sock=folder/'host.sock'
# UDS HTTP is tested with the CLI; the explicit loopback listener uses a separate dev key.
import socket
s=socket.socket();s.bind(('127.0.0.1',0));port=s.getsockname()[1];s.close()
process=subprocess.Popen([str(a.host.resolve()),'--data',str(folder/'data'),'--bundle',str(a.bundle.resolve()),'--web',str(root/'web/dist'),'--socket',str(sock),'--listen',f'127.0.0.1:{port}','--dev-key',key,'--bind','127.0.0.1'],stdout=(folder/'host.log').open('wb'),stderr=subprocess.STDOUT)
base=f'http://127.0.0.1:{port}/app/air2mi';csrf=''
def request(path,method='GET',body=None):
 raw=None if body is None else json.dumps(body).encode();headers={'X-Air2Mi-Host-Key':key,'X-Air2Mi-CSRF':csrf,'Content-Type':'application/json'}
 with urllib.request.urlopen(urllib.request.Request(base+'/api'+path,method=method,data=raw,headers=headers),timeout=65) as r:return json.load(r)
try:
 for _ in range(100):
  if process.poll() is not None:raise RuntimeError((folder/'host.log').read_text())
  try:csrf=request('/session')['csrf'];break
  except OSError:time.sleep(.2)
 else:raise TimeoutError('Host not ready')
 state=request('/state');assert state['engine_running'] is False
 request('/lifecycle/start','POST',{});state=request('/state');assert state['engine_running'];assert state['health']['api_version']=='v1';assert not state['inputs']['inputs']
 # Disabled speaker records exercise actual Registry persistence without a receiver/data plane.
 request('/engine/v1/speakers','POST',{'speaker_id':'integration-only','display_name':'Integration isolated','ip':'127.0.0.1','enabled':False})
 speakers=request('/engine/v1/speakers')['speakers'];assert speakers[0]['display_name']=='Integration isolated'
 request('/engine/v1/speakers/integration-only','PATCH',{'display_name':'Renamed'})
 assert request('/engine/v1/speakers')['speakers'][0]['display_name']=='Renamed'
 request('/engine/v1/inputs','POST',{'display_name':'Simple receiver','enabled':False,'options':{'speaker_id':'integration-only'}})
 receiver=request('/engine/v1/inputs')['inputs'][0];assert receiver['input_id'].startswith('ap-');assert receiver['port']>=7100
 try:
  request('/engine/v1/inputs','POST',{'display_name':'Duplicate target','enabled':False,'options':{'speaker_id':'integration-only'}})
  raise AssertionError('duplicate association accepted')
 except urllib.error.HTTPError as error:
  assert error.code==409,error
 assert len(request('/engine/v1/inputs')['inputs'])==1

 request('/engine/v1/inputs/'+receiver['input_id'],'PATCH',{'display_name':'Renamed receiver','enabled':False,'options':{'speaker_id':'integration-only'}})
 renamed=request('/engine/v1/inputs')['inputs'][0];assert renamed['input_id']==receiver['input_id'] and renamed['port']==receiver['port']
 request('/engine/v1/inputs/'+receiver['input_id'],'DELETE')
 request('/engine/v1/speakers/integration-only','DELETE')
 result=request('/doctor','POST',{});assert result['passed'],result
 request('/lifecycle/restart','POST',{});assert request('/state')['engine_running']
 # Browser smoke uses the same actual stopped Host; no API interception/mock state.
 if os.environ.get('AIR2MI_BROWSER_TEST')=='1':
  subprocess.run(['node','tests/browser.mjs',base,key,str(folder)],check=True,cwd=root)
 request('/lifecycle/stop','POST',{});assert not request('/state')['engine_running']
 print(json.dumps({'passed':True,'folder':str(folder),'tests':['signed install','private API','real Registry CRUD','doctor PCM-to-AAC','restart','stop','UDS status/cleanup']}))
finally:
 subprocess.run([str(a.host.resolve()),'--data',str(folder/'data'),'--socket',str(sock),'--stop'],timeout=60,check=True)
 process.wait(timeout=10)
 assert not sock.exists(),'UDS leaked'
