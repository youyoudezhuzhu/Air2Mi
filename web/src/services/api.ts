declare global { interface Window {
 Air2MiNative?: {requestAsync(id:string,path:string,method:string,body:string):void;pickEngine():void;installUpdate(component:string):void;copyText(value:string):void;exportDiagnostics(value:string):void;backgroundSettings(mode:string):void};
 __air2miReply?: (id:string,response:string)=>void;
} }
export const base=location.pathname.replace(/\/$/,'');
export const nativeAndroid=!!window.Air2MiNative;
let csrf='';const devKey=sessionStorage.getItem('air2mi-dev-key')||'';
export class ApiError extends Error{constructor(public code:string,public status:number,public retryAfter?:number,message=code){super(message)}}
export function chooseAndroidEngine(){window.Air2MiNative?.pickEngine()}
export function openBackgroundSettings(mode='battery'){window.Air2MiNative?.backgroundSettings(mode)}
export async function copyText(value:string){if(window.Air2MiNative){window.Air2MiNative.copyText(value);return}await navigator.clipboard.writeText(value)}
type Reply={status:number;body:any};
const pending=new Map<string,{resolve:(v:Reply)=>void;reject:(e:unknown)=>void;cleanup:()=>void}>();let serial=0;
window.__air2miReply=(id,raw)=>{const request=pending.get(id);if(!request)return;request.cleanup();try{request.resolve(JSON.parse(raw))}catch{request.reject(new ApiError('INVALID_NATIVE_RESPONSE',502))}};
function nativeRequest(path:string,method:string,body:unknown,signal?:AbortSignal):Promise<Reply>{return new Promise((resolve,reject)=>{
 const id=String(++serial),abort=()=>{cleanup();reject(new DOMException('Aborted','AbortError'))},cleanup=()=>{clearTimeout(timer);signal?.removeEventListener('abort',abort);pending.delete(id)},timer=setTimeout(()=>{cleanup();reject(new ApiError('NATIVE_REQUEST_TIMEOUT',504))},45000);
 if(signal?.aborted){abort();return}signal?.addEventListener('abort',abort,{once:true});pending.set(id,{resolve,reject,cleanup});
 try{window.Air2MiNative!.requestAsync(id,'/api'+path,method,body===undefined?'':JSON.stringify(body))}catch(error){cleanup();reject(error)}
})}
export async function api<T=any>(path:string,method='GET',body?:unknown,signal?:AbortSignal):Promise<T>{
 let response:Reply;
 if(window.Air2MiNative)response=await nativeRequest(path,method,body,signal);
 else {const headers:Record<string,string>={};if(devKey)headers['X-Air2Mi-Host-Key']=devKey;if(method!=='GET'){if(!csrf)csrf=(await api<{csrf:string}>('/session')).csrf;headers['X-Air2Mi-CSRF']=csrf}if(body!==undefined&&!(body instanceof Blob))headers['Content-Type']='application/json';const r=await fetch(base+'/api'+path,{method,headers,signal,credentials:'same-origin',body:body===undefined?undefined:body instanceof Blob?body:JSON.stringify(body)});response={status:r.status,body:await r.json()}}
 const value=response.body;if(response.status<200||response.status>=300)throw new ApiError(value.error||'HTTP_ERROR',response.status,value.retry_after,String(value.detail||'').includes('activation_required')?'未激活时最多创建两个 AirPlay 2 实例；请在设置中激活 Air2Mi Plus。':value.error||value.detail||value.reason||value.output||`HTTP ${response.status}`);return value;
}
export async function diagnostics(){const value=await api('/diagnostics');if(window.Air2MiNative){window.Air2MiNative.exportDiagnostics(JSON.stringify(value,null,2));return}const url=URL.createObjectURL(new Blob([JSON.stringify(value,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='air2mi-diagnostics.json';a.click();URL.revokeObjectURL(url)}

export async function downloadAppUpdate(filename:string):Promise<void>{const headers:Record<string,string>={};if(devKey)headers['X-Air2Mi-Host-Key']=devKey;const response=await fetch(base+'/api/app-update-file',{headers,credentials:'same-origin'});if(!response.ok)throw new Error('获取已校验应用包失败');const url=URL.createObjectURL(await response.blob()),link=document.createElement('a');link.href=url;link.download=filename;link.click();setTimeout(()=>URL.revokeObjectURL(url),60000)}
