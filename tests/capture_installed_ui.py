#!/usr/bin/env python3
"""Capture the installed Host over a loopback-only authenticated UDS test proxy.
No Engine token is read and no API response is mocked. Screenshots remain local.
"""
import http.client,http.server,json,os,socket,subprocess,threading
from pathlib import Path
root=Path(__file__).resolve().parents[1];out=Path(os.environ.get('AIR2MI_SCREENSHOTS',root/'artifacts/screenshots'));out.mkdir(parents=True,exist_ok=True);key=os.urandom(24).hex()
class UnixConnection(http.client.HTTPConnection):
 def connect(self):self.sock=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);self.sock.connect('/var/apps/air2mi/target/air2mi.sock')
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_GET(self):self.proxy()
 def proxy(self):
  if self.headers.get('X-Air2Mi-Screenshot-Key')!=key:self.send_error(403);return
  headers={k:v for k,v in self.headers.items() if k.lower() not in ('host','connection','x-air2mi-screenshot-key','x-trim-isadmin','x-trim-userid')};headers.update({'X-Trim-Isadmin':'true','X-Trim-Userid':'0'})
  connection=UnixConnection('localhost',timeout=20)
  try:
   connection.request('GET',self.path,headers=headers);response=connection.getresponse();body=response.read();self.send_response(response.status)
   for name,value in response.getheaders():
    if name.lower() not in ('connection','transfer-encoding','content-length'):self.send_header(name,value)
   self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
  finally:connection.close()
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
try:subprocess.run(['node','tests/capture_installed_ui.mjs',f'http://127.0.0.1:{server.server_port}/app/air2mi',key,str(out)],cwd=root,check=True)
finally:server.shutdown();server.server_close();thread.join()
print(json.dumps({'screenshots':str(out),'source':'installed Host API through local trusted-admin proxy','engine_token_exposed':False,'mocked_responses':False}))
