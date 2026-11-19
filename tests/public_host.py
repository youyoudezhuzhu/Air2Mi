#!/usr/bin/env python3
"""Public CI checks the actual App Host without obtaining or bundling Engine."""
from pathlib import Path
import tempfile,subprocess,os,socket,urllib.request,json,time
root=Path(__file__).resolve().parents[1];temp=Path(tempfile.mkdtemp(prefix='air2mi-public-host-'));bundle=temp/'bundle';bundle.mkdir();(bundle/'trust.pub').write_bytes((root/'engine/development-trust.pub').read_bytes())
s=socket.socket();s.bind(('127.0.0.1',0));port=s.getsockname()[1];s.close();key=os.urandom(24).hex();sock=temp/'app.sock'
process=subprocess.Popen([str(root/'build/air2mi-host'),'--data',str(temp/'data'),'--bundle',str(bundle),'--web',str(root/'web/dist'),'--socket',str(sock),'--listen',f'127.0.0.1:{port}','--dev-key',key],stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
base=f'http://127.0.0.1:{port}/app/air2mi'
try:
 for _ in range(100):
  try:
   with urllib.request.urlopen(urllib.request.Request(base+'/api/state',headers={'X-Air2Mi-Host-Key':key}),timeout=3) as response:state=json.load(response)
   break
  except OSError:
   if process.poll() is not None:raise RuntimeError(process.stderr.read().decode())
   time.sleep(.1)
 else:raise TimeoutError('Host bootstrap failed')
 assert state['engine_installed'] is False and state['engine_running'] is False
 if os.environ.get('AIR2MI_BROWSER_TEST')=='1':subprocess.run(['node','tests/browser_public.mjs',base,key,str(temp)],check=True,cwd=root)
 print(json.dumps({'public_host_without_engine':True,'engine_obtained':False,'data':str(temp)}))
finally:
 subprocess.run([str(root/'build/air2mi-host'),'--data',str(temp/'data'),'--socket',str(sock),'--stop'],check=True,timeout=60)
 process.wait(timeout=10);assert not sock.exists()
