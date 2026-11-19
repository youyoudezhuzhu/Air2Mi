#!/usr/bin/env python3
import argparse,hashlib,io,json,tarfile
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('fpk',type=Path);p.add_argument('--expect-no-engine',action='store_true');a=p.parse_args();lock=json.loads(Path('engine/manifest.json').read_text())
with tarfile.open(a.fpk) as package:
 members={m.name.lstrip('./'):m for m in package};required={'manifest','ICON.PNG','ICON_256.PNG','app.tgz','cmd/main','config/privilege','config/resource'};assert required<=members.keys(),required-members.keys()
 for name in ['main','install_init','install_callback','upgrade_init','upgrade_callback','uninstall_init','uninstall_callback','config_init','config_callback']:assert members['cmd/'+name].mode&0o111,name
 payload=package.extractfile(members['app.tgz']).read()
 with tarfile.open(fileobj=io.BytesIO(payload),mode='r:gz') as app:
  content={m.name.lstrip('./'):m for m in app}
  binaries=[k for k in content if k.endswith('engine/engine.run')]
  if a.expect_no_engine:assert not binaries,'Public FPK contains Engine'
  elif binaries:
   binary=app.extractfile(content[binaries[0]]).read();assert hashlib.sha256(binary).hexdigest()==lock['sha256']
  assert not any(k.endswith(('sender.py','rtsp.py','tsmux.py','.so','.whl')) for k in content),'Engine/native payload in App archive'
  assert any(k.endswith('web/index.html') for k in content);assert any(k.endswith('bin/air2mi-host') for k in content)
  for k in content:assert not k.endswith(('.key','api-token','host-control.token')),k
 print('FPK format, lifecycle scripts, UI and Engine inclusion policy verified')
