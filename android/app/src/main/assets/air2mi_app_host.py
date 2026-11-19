"""Android management adapter. Audio, discovery and scheduling stay in imported Engine."""
import contextlib,datetime,hashlib,ipaddress,json,math,re,shutil,socket,struct,threading,time,uuid,wave
from pathlib import Path
from urllib import request,error,parse

class Failure(Exception):
    def __init__(self,status,code,**fields):self.status=status;self.body={'error':code,**fields};super().__init__(code)

def display_text(value):
    def replace(match):
        raw=re.sub('-','',match.group()[5:]).upper()
        if not re.fullmatch('[0-9A-F]{32}',raw):return '[device-code-unavailable]'
        mask=hashlib.sha256(b'air2mi.device.display.v1').digest()
        code=bytes(a^b for a,b in zip(bytes.fromhex(raw),mask)).hex().upper()
        return 'AVD1-'+'-'.join(code[n:n+4] for n in range(0,32,4))
    text=re.sub(r'(?i)AMD1-[A-Z0-9-]+',replace,str(value))
    return re.sub(r'(?i)A2M-(?:[A-F0-9]{5}-){7}[A-F0-9]{5}','[redacted]',text)

def public(value):
    if isinstance(value,dict):
        return {k:public(v) for k,v in value.items() if k.lower() not in ('token','authorization','license_code','signature','payload','credential')}
    if isinstance(value,list):return [public(v) for v in value]
    return display_text(value) if isinstance(value,str) else value

def idle(state):
    if not isinstance(state.get('river'),dict) or not isinstance(state.get('instances'),dict):raise Failure(409,'无法确定当前播放状态')
    if state['river'].get('producer') or any(v.get('selected') for v in state['instances'].values()) or any(v.get('connected') for v in state.get('speakers',[])):
        raise Failure(409,'请先取消所有 AirPlay 投放，再播放测试音')

def validate_associations(inputs,routes,speakers):
    known={s['speaker_id'] for s in speakers};names={i['input_id']:i['display_name'] for i in inputs};owners={};targets={}
    def claim(i,s):
        if i not in names or s not in known:raise Failure(422,'关联的接收器或音箱不存在')
        if i in targets and targets[i]!=s:raise Failure(409,'每个接收器只能关联一台音箱')
        if s in owners and owners[s]!=i:raise Failure(409,'该音箱已关联接收器「'+names[owners[s]]+'」')
        owners[s]=i;targets[i]=s
    for i in inputs:
        target=i.get('options',{}).get('speaker_id')
        if target:claim(i['input_id'],target)
    for route in routes:
        if len(route.get('input_ids',[]))!=1 or len(route.get('speaker_ids',[]))!=1:raise Failure(422,'每条关联仅支持一个接收器和一台音箱')
        claim(route['input_ids'][0],route['speaker_ids'][0])

def validate_offer(v):
    if not isinstance(v,dict) or v.get('schema_version')!=1 or v.get('app_id')!='air2mi' or v.get('product_id') not in ('plus','air2mi_plus'):return False
    if type(v.get('enabled')) is not bool or type(v.get('purchasable')) is not bool or not re.fullmatch('[A-Z]{3}',v.get('currency','')):return False
    for key in ('price_minor','original_price_minor'):
        if v.get(key) is not None and (type(v[key]) is not int or not 0<=v[key]<=9007199254740991):return False
    for key in ('title','description','button_text','purchase_url','badge','updated_at'):
        if v.get(key) is not None and not isinstance(v[key],str):return False
    for f in v.get('features') or []:
        if not isinstance(f,dict) or not isinstance(f.get('id'),str) or not isinstance(f.get('title'),str):return False
    return True


def near_uuid(a,b):
    try:a=str(uuid.UUID(a));b=str(uuid.UUID(b))
    except (ValueError,TypeError,AttributeError):return False
    return 0<sum(x!=y for x,y in zip(a,b))<=2

def discovery_match(item,known,references):
    key=item.get('options',{}).get('discovery_id','')
    if not key:return next((s for s in known if s['speaker_id']==item['speaker_id']),None),[]
    exact=[s for s in known if s.get('options',{}).get('discovery_id')==key]
    related=[s for s in known if s in exact or near_uuid(s.get('options',{}).get('discovery_id'),key)]
    active=[s for s in related if references.get(s['speaker_id'],0)>0]
    if len(related)>1 and len(active)==1:return active[0],exact
    if len(exact)==1:return exact[0],exact
    if len(related)==1 and len(active)==1:return active[0],exact
    if len(exact)>1:raise Failure(409,'发现身份冲突，请检查已有音箱记录')
    return None,exact

class Offer:
    URL='https://license.yiling.win/api/v1/public/apps/air2mi/offers/plus'
    def __init__(self,opener=request.urlopen,clock=time.monotonic):self.open=opener;self.clock=clock;self.value=None;self.etag='';self.until=0;self.lock=threading.Lock()
    def get(self):
        with self.lock:
            now=self.clock()
            if self.value is not None and now<self.until:return self.value
            headers={'Accept':'application/json','User-Agent':'Air2Mi-Android/1.1.0'}
            if self.etag:headers['If-None-Match']=self.etag
            try:
                try:response=self.open(request.Request(self.URL,headers=headers),timeout=10)
                except error.HTTPError as e:response=e
                with response:
                    status=response.status
                    if status==304 and self.value is not None:self.until=now+60;return self.value
                    if status==429:
                        try:retry=max(1,min(86400,int(response.headers.get('Retry-After','60'))))
                        except ValueError:retry=60
                        raise Failure(429,'RATE_LIMITED',retry_after=retry)
                    raw=response.read(131073)
                    if len(raw)>131072:raise Failure(502,'OFFER_INVALID_RESPONSE')
                    value=json.loads(raw)
                    if status!=200:raise Failure(404 if value.get('code')=='OFFER_NOT_CONFIGURED' else 502,value.get('code','OFFER_UNAVAILABLE'))
                    if not validate_offer(value):raise Failure(502,'OFFER_INVALID_RESPONSE')
                    self.value=value;self.etag=response.headers.get('ETag','');self.until=now+60;return value
            except Failure:raise
            except Exception:raise Failure(502,'OFFER_UNAVAILABLE') from None

class Manager:
    def __init__(self,owner,data_dir,native_dir):
        self.owner=owner;self.data=Path(data_dir);self.native=Path(native_dir);self.lock=threading.RLock();self.offers=Offer();self.tone=Tone(self)
    def call(self,path,method='GET',body=None):
        envelope=json.loads(str(self.owner.callEngine(path,method,json.dumps(body) if body is not None else '')))
        if envelope['status']>=400:
            value=envelope['body'];raise Failure(envelope['status'],value.get('error','ENGINE_ERROR'),**{k:v for k,v in value.items() if k!='error'})
        return envelope['body']
    def stop_tone(self):return self.tone.stop()
    def configuration(self):
        try:return json.loads((self.data/'registry.json').read_text())
        except FileNotFoundError:return {'inputs':[],'speakers':[],'routes':[]}
    def request(self,path,method,raw):
        try:
            body=json.loads(raw) if raw else {}
            if not isinstance(body,dict):raise Failure(400,'请求必须为 JSON 对象')
            value=self.dispatch(path,method,body)
            return json.dumps({'status':200,'body':public(value)},ensure_ascii=False)
        except Failure as e:return json.dumps({'status':e.status,'body':public(e.body)},ensure_ascii=False)
        except Exception as e:return json.dumps({'status':422,'body':{'error':display_text(str(e))}},ensure_ascii=False)
    def dispatch(self,path,method,body):
        parsed=parse.urlsplit(path);path=parsed.path
        if path=='/api/plus/offers' and method=='GET':return self.offers.get()
        if path=='/api/test-tone' and method=='GET':return self.tone.snapshot()
        if path=='/api/test-tone/stop' and method=='POST':return self.tone.stop()
        match=re.fullmatch('/api/speakers/([^/]+)/test-tone',path)
        if match and method=='POST':return self.tone.start(parse.unquote(match[1]),body)
        if path=='/api/discovery' and method=='POST':
            if body:raise Failure(400,'搜索不接受自定义路径或命令')
            return self.discover()
        if path=='/api/logs' and method=='GET':return self.logs(parse.parse_qs(parsed.query))
        if path.startswith('/api/engine/v1/'):
            target='/v1/'+path[len('/api/engine/v1/'):]
            if not re.fullmatch(r'/v1/(health|state|config|river|inputs(?:/[^/]+)?|speakers(?:/[^/]+)?|routes(?:/[^/]+)?|license(?:/(?:activate|restore|validate|clear))?)',target):raise Failure(404,'接口未开放')
            if target=='/v1/license/clear' and method=='POST':self.tone.stop()
            if method!='GET' and not target.startswith('/v1/license'):
                with self.lock:
                    if self.tone.busy():raise Failure(409,'请先停止测试音')
                    return self.mutate(target,method,body)
            return self.call(target,method,body if method!='GET' else None)
        raise Failure(404,'接口不存在')
    def mutate(self,path,method,body):
        parts=path.split('/');kind=parts[2];identity=parse.unquote(parts[3]) if len(parts)>3 else None
        if kind not in ('inputs','speakers','routes'):raise Failure(405,'不支持的操作')
        config={key:self.call('/v1/'+key).get(key,[]) for key in ('inputs','speakers','routes')}
        if method=='POST':
            if identity is not None:raise Failure(405,'不支持的操作')
            if kind=='inputs':
                if set(body)-{'kind','display_name','enabled','options'}:raise Failure(400,'接收器标识和端口由应用管理')
                body=dict(body,input_id='ap-'+uuid.uuid4().hex[:16],kind='airplay')
                occupied={i.get('port') for i in config['inputs']}
                for port in range(7100,8100):
                    if port in occupied:continue
                    try:
                        with socket.socket() as probe:probe.bind(('0.0.0.0',port))
                        body['port']=port;break
                    except OSError:continue
                else:raise Failure(409,'没有可用的接收器端口')
            elif kind=='speakers' and not body.get('speaker_id'):body=dict(body,speaker_id='sp-'+uuid.uuid4().hex[:16])
            key={'inputs':'input_id','speakers':'speaker_id','routes':'route_id'}[kind]
            identity=body.get(key)
            config[kind].append(body)
        elif method=='PATCH':
            key={'inputs':'input_id','speakers':'speaker_id','routes':'route_id'}[kind]
            config[kind]=[{**item,**body} if item.get(key)==identity else item for item in config[kind]]
        elif method=='DELETE':
            # Engine remains authoritative for reference protection and deletion.
            return self.call(path,method,body)
        else:raise Failure(405,'不支持的操作')
        if kind in ('inputs','routes'):validate_associations(config['inputs'],config['routes'],config['speakers'])
        if kind=='inputs' and not next((i.get('options',{}).get('speaker_id') for i in config['inputs'] if i.get('input_id')==identity),None):raise Failure(422,'请选择关联音箱')
        return self.call(path,method,body)
    def discover(self):
        with self.lock:
            if self.tone.busy():raise Failure(409,'请先停止测试音')
            known=self.call('/v1/speakers')['speakers'];inputs=self.call('/v1/inputs')['inputs'];routes=self.call('/v1/routes')['routes']
            from tools.miplay_mdns_discovery import discover
            from tools.daily_kernel import discovered_registry
            bind=str(self.owner.getBindAddress() or '')
            if not bind:raise Failure(409,'未找到可用的局域网 IPv4 网络')
            devices=discover(timeout=5,bind=bind)
            if not devices:return {'discovered':0,'added':0,'updated':0,'skipped':0,'bind':bind,'speakers':known}
            found=discovered_registry(devices,{'schema_version':1,'inputs':inputs,'speakers':known,'routes':routes})['speakers']
            references={s:sum(i.get('options',{}).get('speaker_id')==s for i in inputs)+sum(s in r.get('speaker_ids',[]) for r in routes) for s in [k['speaker_id'] for k in known]}
            added=updated=skipped=0
            for item in found:
                old,matches=discovery_match(item,known,references)
                if old:
                    changes={k:item[k] for k in ('ip','model')};changes['options']={**old.get('options',{}),**item.get('options',{})}
                    if any(old.get(k)!=v for k,v in changes.items()):self.call('/v1/speakers/'+parse.quote(old['speaker_id'],safe=''),'PATCH',changes);old.update(changes);updated+=1
                    else:skipped+=1
                    for duplicate in matches:
                        if duplicate is not old and references.get(duplicate['speaker_id'],0)==0:self.call('/v1/speakers/'+parse.quote(duplicate['speaker_id'],safe=''),'DELETE',{});known.remove(duplicate)
                elif any(s['ip']==item['ip'] for s in known):skipped+=1
                else:self.call('/v1/speakers','POST',item);known.append(item);added+=1
            return {'discovered':len(found),'added':added,'updated':updated,'skipped':skipped,'bind':bind,'speakers':self.call('/v1/speakers')['speakers']}
    def logs(self,query):
        needle=query.get('q',[''])[0][:200];level=query.get('level',['all'])[0]
        paths=sorted(self.data.glob('runs/**/bridge.log'),key=lambda p:p.stat().st_mtime)[-3:]
        paths+=[self.data/'app.log']
        lines=[]
        for path in paths:
            if not path.is_file():continue
            with path.open('rb') as stream:stream.seek(max(0,path.stat().st_size-131072));text=stream.read(131072).decode('utf8','replace')
            for line in text.splitlines():
                line=display_text(re.sub(r'(?i)(Bearer\s+\S+|A2M-(?:[A-F0-9]{5}-){7}[A-F0-9]{5})','[redacted]',line))
                if needle.lower() in line.lower() and (level=='all' or level in line.lower()):lines.append(line)
        return {'text':'\n'.join(lines[-1500:])}

class Tone:
    def __init__(self,manager):self.m=manager;self.lock=threading.RLock();self.state={'phase':'idle'};self.cancel=threading.Event();self.thread=None;self.child=None;self.cleanup_failure=False
    def ensure_clean(self):
        if self.cleanup_failure:raise Failure(409,'上次测试音尚未清理完成，不能启动其他播放')
    def snapshot(self):
        with self.lock:return dict(self.state)
    def busy(self):return self.cleanup_failure or (self.thread is not None and self.thread.is_alive())
    def phase(self,phase,**fields):
        with self.lock:self.state.update(phase=phase,changed_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),**fields)
    def start(self,speaker_id,body):
        if set(body)!={'volume_percent'} or type(body['volume_percent']) is not int or not 1<=body['volume_percent']<=100:raise Failure(400,'测试仅接受 1–100 的整数音量，不接受文件路径')
        with self.m.lock:
            if self.busy():raise Failure(409,'已有测试音正在运行')
            idle(self.m.call('/v1/state'))
            target=next((s for s in self.m.call('/v1/speakers')['speakers'] if s['speaker_id']==speaker_id and s.get('enabled')),None)
            if target is None:raise Failure(404,'未找到启用的目标音箱')
            self.cancel.clear();self.state={'phase':'connecting','speaker_id':speaker_id,'id':'tone-'+uuid.uuid4().hex,'device_volume':body['volume_percent'],'max_seconds':300}
            self.phase('connecting');self.thread=threading.Thread(target=self.run,args=(dict(target),body['volume_percent']),daemon=True,name='air2mi-app-test-tone');self.thread.start();return self.snapshot()
    def stop(self,wait=True):
        thread=self.thread
        if thread is None or not thread.is_alive():
            if self.cleanup_failure:
                self.child.shutdown();self.child=None;self.cleanup_failure=False;self.m.owner.resumeAfterTone();self.phase('stopped')
            return self.snapshot()
        self.cancel.set();self.phase('stopping')
        if wait:
            thread.join(40)
            if thread.is_alive():raise Failure(409,'测试音正在清理，请稍候')
        return self.snapshot()
    def run(self,target,volume):
        child=None;folder=None;failure=None;suspended=False
        try:
            from dataclasses import replace
            from a2m_kernel.host import EngineHost
            from engine_runtime.configuration import configure_host
            self.m.owner.suspendForTone();suspended=True
            folder=self.m.data.parent/'test-tone'/self.state['id'];folder.mkdir(parents=True,exist_ok=False)
            asset=folder/'fixed-test-tone.wav';write_tone(asset)
            cfg=replace(configure_host(folder,self.m.native,False,str(self.m.owner.getBindAddress())),daily_mode=False)
            child=EngineHost();self.child=child;child.initialize(cfg,str(folder),platform='android',native_dir=str(self.m.native),android_host=self.m.owner,embedded=True);child.start()
            endpoint=child.get_api_endpoint()
            def call(path,body):
                req=request.Request(endpoint['url']+path,data=json.dumps(body).encode(),headers={'Authorization':'Bearer '+endpoint['token'],'Content-Type':'application/json'},method='POST')
                with request.urlopen(req,timeout=30) as response:return json.load(response)
            call('/v1/speakers',{**target,'volume':volume})
            call('/v1/sessions',{'session_id':self.state['id'],'display_name':'Air2Mi 测试音','kind':'file','speaker_ids':[target['speaker_id']],'file':str(asset),'loop':False})
            start=time.monotonic();first=False
            while not self.cancel.wait(.25):
                state=child.status()['state'];observations=state.get('speakers',[])
                output=next((o for o in observations if o.get('speaker_id')==target['speaker_id']),{})
                if output.get('first_audio_pcm') or output.get('first_audio_confirmed'):
                    if not first:self.phase('playing',first_audio_confirmed=True);first=True
                if time.monotonic()-start>300:break
                if not first and time.monotonic()-start>20:raise RuntimeError('音箱未在限定时间确认首帧音频')
            self.phase('stopping')
            call('/v1/sessions/'+self.state['id']+'/stop',{})
        except Exception as e:failure=display_text(str(e))
        finally:
            if child is not None:
                try:child.shutdown()
                except Exception as e:failure=display_text(str(e));self.cleanup_failure=True
            # Do not resume the daily profile until all child native tasks are stopped.
            if suspended and not self.cleanup_failure and (child is None or not child.status().get('running')):
                try:self.m.owner.resumeAfterTone()
                except Exception as e:failure=display_text(str(e))
            if not self.cleanup_failure:
                self.child=None
                if folder is not None:
                    try:shutil.rmtree(folder)
                    except OSError as error:failure=display_text(str(error))
            self.phase('failed' if failure else 'stopped',error=failure or '')

def write_tone(path):
    rate=44100;cycle=bytearray(rate*4*4)
    for sample in range(rate*4):
        t=sample/rate;slot=int(t/.8);local=t-slot*.8;value=0
        if slot<3 and local<.30:value=int(32767*.12*min(1,local/.015,(.30-local)/.015)*math.sin(2*math.pi*(440,660,880)[slot]*local))
        struct.pack_into('<hh',cycle,sample*4,value,value)
    with wave.open(str(path),'wb') as out:
        out.setnchannels(2);out.setsampwidth(2);out.setframerate(rate)
        for _ in range(83):out.writeframesraw(cycle)

def public_json(value):return json.dumps(public(json.loads(value)),ensure_ascii=False)