#!/usr/bin/env python3
import argparse,hashlib,json,os,shutil,subprocess,tempfile
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument('--engine',type=Path,help='Local-only signed Engine bundle; omitted in public CI')
parser.add_argument('--fnpack',default='fnpack')
parser.add_argument('--output',type=Path,default=Path('dist'))
parser.add_argument('--skip-icons',action='store_true',help='use checked-in fnOS icons when frontend tooling is unavailable')
a=parser.parse_args()
root=Path(__file__).resolve().parents[1];os.chdir(root)
lock=json.loads(Path('engine/manifest.json').read_text())
if a.engine:
 for name in ['engine.run','manifest.json','manifest.sig','trust.pub']:
  if not (a.engine/name).is_file():raise SystemExit('Missing compiled bundle '+name)
 if hashlib.sha256((a.engine/'engine.run').read_bytes()).hexdigest()!=lock['sha256']:raise SystemExit('Engine hash mismatch')
 if hashlib.sha256((a.engine/'manifest.json').read_bytes()).hexdigest()!=lock['manifest_sha256']:raise SystemExit('Manifest hash mismatch')
 if (a.engine/'trust.pub').read_text().strip()!=lock['public_key']:raise SystemExit('Unexpected trust anchor')
subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o','build/air2mi-host','./hosts/fnos'],check=True,env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64'))
if not a.skip_icons: subprocess.run(['node','tools/icons.mjs'],check=True)
a.output.mkdir(exist_ok=True,parents=True)
# fnpack mutates staging ACLs: never run it against the checkout.
with tempfile.TemporaryDirectory(prefix='air2mi-fpk-') as tmp:
 stage=Path(tmp)/'package';shutil.copytree(root/'packaging/fnos',stage)
 shutil.copytree(root/'web/dist',stage/'app/web')
 (stage/'app/bin').mkdir();shutil.copy2('build/air2mi-host',stage/'app/bin/air2mi-host')
 if a.engine:shutil.copytree(a.engine,stage/'app/engine')
 else:
  (stage/'app/engine').mkdir();shutil.copy2('engine/development-trust.pub',stage/'app/engine/trust.pub')
 shutil.copy2('engine/manifest.json',stage/'app/engine-lock.json')
 (stage/'app/licenses').mkdir();shutil.copy2('LICENSE',stage/'app/licenses/APP-MIT.txt');shutil.copy2('engine/README.md',stage/'app/licenses/ENGINE-NOTICE.md')
 try:
  subprocess.run([a.fnpack,'build','-d',str(stage)],check=True,cwd=tmp)
  files=list(stage.glob('*.fpk'))+list(Path(tmp).glob('*.fpk'))
  if len(files)!=1:raise RuntimeError('Expected exactly one new FPK')
  target=a.output/'Air2Mi.fpk';shutil.copyfile(files[0],target)
 finally:
  subprocess.run(['chmod','-R','u+rwX',tmp],check=False)
 digest=hashlib.sha256(target.read_bytes()).hexdigest();(a.output/'SHA256SUMS').write_text(digest+'  Air2Mi.fpk\n')
 print(json.dumps({'fpk':str(target),'sha256':digest,'engine_included':bool(a.engine),'engine_sha256':lock['sha256'] if a.engine else None}))
