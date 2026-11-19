"""Authenticated Android APK transport; APK replacement remains an OS operation."""
import base64,hashlib,json,os,threading,time
from pathlib import Path
from urllib import request,parse
SOURCE='https://youyoudezhuzhu.github.io/Air2Mi/updates/engine-stable.json'
MAX_APK=384*1024*1024

class UpdateError(ValueError):pass
def app_number(value):
 parts=value.split('-',1)[0].split('.')
 if len(parts)!=3 or any(not p.isascii() or not p.isdecimal() for p in parts):raise UpdateError('应用版本无效')
 return tuple(map(int,parts))
def decode(value):
 if not isinstance(value,str) or not value or len(value)>350000:raise UpdateError('更新编码无效')
 try:raw=base64.b64decode(value+'='*(-len(value)%4),altchars=b'-_',validate=True)
 except Exception:raise UpdateError('更新编码无效') from None
 if base64.urlsafe_b64encode(raw).decode().rstrip('=')!=value:raise UpdateError('更新编码不规范')
 return raw
def verify(raw,key,component="engine"):
 from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
 if len(raw)>256*1024:raise UpdateError('更新清单过大')
 try:
  envelope=json.loads(raw)
  if envelope['format']!='air2mi-update-v1' or envelope['key_id']!='air2mi-update-v1':raise ValueError()
  payload=decode(envelope['payload']);signature=decode(envelope['signature'])
  if len(signature)!=64:raise ValueError()
  Ed25519PublicKey.from_public_bytes(key).verify(signature,payload)
  value=json.loads(payload)
  if (value['schema_version']!=1 or type(value['schema_version']) is not int or value['app_id']!='air2mi' or value.get('component')!=component or value['channel']!='stable' or
      type(value['sequence']) is not int or value['sequence']<0 or type(value['published']) is not bool or len(value.get('targets',[]))>8):raise ValueError()
  return value
 except Exception:raise UpdateError('更新清单签名或内容无效') from None

class Client:
 def __init__(self,folder,public_key,package_name,version_code,source=None,test_http=False,component="engine",app_version=""):
  if component not in ("engine","app"):raise UpdateError("更新组件无效")
  self.component=component;self.app_version=app_version
  source=source or SOURCE.replace("engine-stable",component+"-stable")
  self.folder=Path(folder);self.folder.mkdir(parents=True,exist_ok=True)
  self.key=base64.b64decode(public_key);self.package=package_name;self.version=int(version_code);self.source=source;self.test_http=test_http
  if len(self.key)!=32:raise UpdateError('更新信任公钥无效')
  self.lock=threading.RLock();self.target=None;self.working=False
  self.state=dict(configured=True,phase='idle',available=False,reason='尚未检查稳定版本',kind='apk',component=self.component)
 def allowed(self,url):
  u=parse.urlsplit(url)
  if self.test_http and url.startswith(self.source.rsplit('/',1)[0]+'/'):return url
  if u.scheme!='https' or u.username or u.password or u.fragment or u.port:raise UpdateError('更新只允许可信 HTTPS 地址')
  if ((u.hostname=='youyoudezhuzhu.github.io' and u.path.startswith('/Air2Mi/')) or
      (u.hostname=='github.com' and u.path.startswith('/youyoudezhuzhu/Air2Mi/releases/download/')) or u.hostname=='release-assets.githubusercontent.com'):return url
  raise UpdateError('更新地址不在发布源白名单')
 def open(self,url):
  client=self
  class Redirect(request.HTTPRedirectHandler):
   def redirect_request(self,req,fp,code,msg,headers,newurl):client.allowed(newurl);return super().redirect_request(req,fp,code,msg,headers,newurl)
  return request.build_opener(Redirect()).open(request.Request(self.allowed(url),headers={'Cache-Control':'no-cache'}),timeout=12)
 def snapshot(self):
  with self.lock:return json.dumps(self.state,ensure_ascii=False)
 def check(self,engine_manifest='{}'):
  with self.lock:
   if self.working:return self.snapshot()
  try:
   with self.open(self.source) as response:
    if response.status!=200:raise UpdateError('更新源 HTTP '+str(response.status))
    catalog=verify(response.read(256*1024+1),self.key,self.component)
   current=json.loads(engine_manifest).get('package_version','') if self.component=='engine' else self.app_version
   state=dict(component=self.component,current_version=current,configured=True,phase='current',available=False,reason='已是当前稳定版本',kind='apk')
   target=None
   if not catalog['published']:state.update(phase='unpublished',reason=catalog.get('reason') or '暂未发布可用稳定版')
   else:
    targets=[t for t in catalog['targets'] if t.get('platform')=='android' and t.get('architecture')=='arm64-v8a']
    if len(targets)!=1:raise UpdateError('没有唯一的 Android ARM64 稳定目标')
    target=targets[0]
    components=self.component=='engine' and target.get('kind')=='engine_components'
    if not components and (target.get('kind')!='apk' or target.get('api_version')!='v1' or target.get('application_id')!=self.package):raise UpdateError('更新包不适用于当前应用；测试版与正式版不能覆盖')
    if target.get('api_version')!='v1':raise UpdateError('更新接口版本不兼容')
    if (not components and type(target.get('version_code')) is not int) or type(target.get('size')) is not int or not 0<target['size']<=MAX_APK:raise UpdateError('APK 版本或大小无效')
    for name in (('sha256',) if components else ('sha256','signer_sha256')):
     if len(target.get(name,''))!=64 or any(c not in '0123456789abcdef' for c in target[name]):raise UpdateError('APK 校验字段无效')
    if len(target.get('source_commit',''))!=40 or not target.get('package_version'):raise UpdateError('内核版本信息缺失')
    self.allowed(target['url'])
    if target.get('url_alt'):self.allowed(target['url_alt'])
    marker=self.folder/'accepted-sequence.json';highest=json.loads(marker.read_text()).get('sequence',0) if marker.exists() else 0
    if catalog['sequence']<highest:raise UpdateError('拒绝旧版本更新清单')
    state.update(app_version=target.get('app_version',''),package_version=target['package_version'],kernel_version=target['kernel_version'],release_id=str(catalog['sequence'])+':'+target['sha256'],total=target['size'],downloaded=0)
    if not components and not target.get('app_version'):raise UpdateError('应用版本信息缺失')
    if not components and self.app_version and app_number(target['app_version'])<app_number(self.app_version):raise UpdateError('内核更新不能降级应用')
    changed=target['package_version']!=current if self.component=='engine' else target['app_version']!=current
    state['kind']=target['kind']
    if changed and (components or target['version_code']>self.version):state.update(phase='available',available=True,reason='发现可安装的'+('内核' if self.component=='engine' else '应用')+'更新')
    elif changed and self.component=='engine':state.update(phase='requires_new_package',reason='内核版本不同，但需要发布更高安装版本号的 APK 才能覆盖更新')
    else:target=None
   with self.lock:
    if not self.working:self.target=target;self.state=state;self.sequence=catalog['sequence']
   return self.snapshot()
  except Exception as error:
   with self.lock:
    if not self.working:self.state=dict(configured=True,phase='error',available=False,reason=str(error),kind='apk')
   return self.snapshot()
 def begin(self,release_id):
  with self.lock:
   if self.working or not self.state.get('available') or self.state.get('release_id')!=release_id:raise UpdateError('版本已变化或正在更新，请重新检查')
   self.working=True;self.state.update(phase='downloading',available=False,reason='正在下载并校验 APK')
 def download(self):
  target=self.target
  try:
   if not self.working or not target:raise UpdateError('没有已确认的更新任务')
   components=target.get('kind')=='engine_components'
   errors=[];output=self.folder/(self.component+('-update.zip' if components else '-update.apk'));pending=self.folder/(self.component+'-update.partial')
   for url in (target['url'],target.get('url_alt')):
    if not url:continue
    try:
     total=0;digest=hashlib.sha256();started=time.monotonic()
     with self.open(url) as response,pending.open('wb') as dest:
      if response.status!=200:raise UpdateError('下载 HTTP '+str(response.status))
      while True:
       if time.monotonic()-started>300:raise UpdateError('下载超时')
       chunk=response.read(64*1024)
       if not chunk:break
       total+=len(chunk)
       if total>target['size']:raise UpdateError('APK 大小超出签名清单')
       dest.write(chunk);digest.update(chunk)
       with self.lock:self.state['downloaded']=total
      dest.flush();os.fsync(dest.fileno())
     if total!=target['size'] or digest.hexdigest()!=target['sha256']:raise UpdateError('APK SHA256 或大小不匹配')
     os.replace(pending,output)
     marker=self.folder/'accepted-sequence.json';temporary=marker.with_suffix('.new')
     with temporary.open('w') as record:json.dump({'sequence':self.sequence},record);record.flush();os.fsync(record.fileno())
     os.replace(temporary,marker)
     with self.lock:self.state.update(phase='installing' if components else 'install_ready',reason='正在导入内核' if components else '下载完成，请确认系统覆盖安装',available=False,system_install_required=not components)
     return str(output)
    except Exception as error:errors.append(str(error));pending.unlink(missing_ok=True)
   raise UpdateError('; '.join(errors))
  except Exception as error:
   with self.lock:self.state.update(phase='error',reason=str(error))
   raise
  finally:
   with self.lock:self.working=False
 def imported(self,restart):
  with self.lock:
   self.target=None
   self.state.update(phase='installed',available=False,reason='内核已导入，请重新启动应用' if restart else '内核已安装')
 def candidate(self):return json.dumps(self.target or {},ensure_ascii=False)
 def failed(self,message):
  with self.lock:self.working=False;self.state.update(phase='error',reason=message,available=False)
