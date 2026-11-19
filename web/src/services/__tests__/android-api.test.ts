import {afterEach,beforeEach,expect,it,vi} from 'vitest';
beforeEach(()=>{vi.resetModules();vi.stubGlobal('location',{pathname:'/assets/index.html'});vi.stubGlobal('sessionStorage',{getItem:()=>null});vi.stubGlobal('window',{});});
afterEach(()=>{vi.unstubAllGlobals()});
it('native requests are asynchronous and preserve engine errors and Retry-After',async()=>{
 const calls:any[]=[];(window as any).Air2MiNative={requestAsync:(...args:any[])=>calls.push(args)};
 const {api,ApiError}=await import('../api');const result=api('/engine/v1/license/restore','POST',{});expect(calls.length).toBe(1);
 window.__air2miReply!(calls[0][0],JSON.stringify({status:429,body:{error:'RATE_LIMITED',retry_after:37}}));
 await expect(result).rejects.toMatchObject({code:'RATE_LIMITED',status:429,retryAfter:37});expect(ApiError).toBeDefined();
});
it('aborting a page request removes its callback without invoking another request',async()=>{
 const calls:any[]=[];(window as any).Air2MiNative={requestAsync:(...args:any[])=>calls.push(args)};const {api}=await import('../api');
 const controller=new AbortController();const result=api('/state','GET',undefined,controller.signal);controller.abort();await expect(result).rejects.toMatchObject({name:'AbortError'});
 expect(()=>window.__air2miReply!(calls[0][0],JSON.stringify({status:200,body:{}}))).not.toThrow();expect(calls.length).toBe(1);
});
it('native clipboard and diagnostics export use the platform adapter',async()=>{
 const copy=vi.fn(),save=vi.fn();(window as any).Air2MiNative={copyText:copy,exportDiagnostics:save,requestAsync:(id:string)=>queueMicrotask(()=>window.__air2miReply!(id,JSON.stringify({status:200,body:{ok:true}})))};
 const {copyText,diagnostics}=await import('../api');await copyText('AVD1-test');await diagnostics();expect(copy).toHaveBeenCalledWith('AVD1-test');expect(JSON.parse(save.mock.calls[0][0])).toEqual({ok:true});
});
