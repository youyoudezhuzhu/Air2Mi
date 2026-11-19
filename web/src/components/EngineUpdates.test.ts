import {describe,it,expect,vi} from 'vitest';
import {createSSRApp,h} from 'vue';
import {renderToString} from '@vue/server-renderer';
import {createPinia} from 'pinia';
import {useEngine} from '../stores/engine';
vi.mock('../services/api',()=>({api:vi.fn(),base:'',nativeAndroid:false,downloadAppUpdate:vi.fn()}));
import EngineUpdates from './EngineUpdates.vue';
describe('Independent App and Engine version cards',()=>{
 it('renders both installed versions without inventing a remote update',async()=>{
  const pinia=createPinia(),store=useEngine(pinia);
  store.snapshot={app_version:'1.1.1',engine_manifest:{package_version:'0.3.11-stable.1-hardened'}} as any;
  const app=createSSRApp({render:()=>h('div',[h(EngineUpdates,{component:'app'}),h(EngineUpdates,{component:'engine'},{offline:()=>h('button','离线导入')})])});app.use(pinia);
  const html=await renderToString(app);
  expect(html).toContain('data-update-component="app"');expect(html).toContain('data-update-component="engine"');
  expect(html).toContain('当前版本 1.1.1');expect(html).toContain('当前版本 0.3.11-stable.1-hardened');
  expect(html.match(/在线检查更新/g)?.length).toBe(2);
  expect(html.match(/内核安装与更新/g)?.length).toBe(1);
  expect(html.match(/离线导入/g)?.length).toBe(1);
 expect(html).not.toContain('已是最新');expect(html).not.toContain('确认系统安装');
 });
});
