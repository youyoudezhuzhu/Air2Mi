import {createServer} from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {chromium} from '../../web/node_modules/playwright/index.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../web/dist');
const report=process.env.AIR2MI_UI_REPORT||path.resolve('artifacts/android-ui');await fs.mkdir(report,{recursive:true});
const server=createServer(async(req,res)=>{try{const pathname=decodeURIComponent(new URL(req.url,'http://localhost').pathname);const file=path.resolve(root,'.'+(pathname==='/'?'/index.html':pathname));if(!file.startsWith(root+path.sep))throw Error('path');const data=await fs.readFile(file);res.setHeader('Content-Type',file.endsWith('.js')?'application/javascript':file.endsWith('.css')?'text/css':'text/html');res.end(data)}catch{res.writeHead(404);res.end()}});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/opt/google/chrome/chrome',headless:true,args:['--no-sandbox']});const results=[];
try{for(const width of [390,1280]){
 const page=await browser.newPage({viewport:{width,height:900}});await page.addInitScript(()=>{
  // Test fixture only: Android transport/UI contract, not an audio hardware test.
  const state={app_version:'1.1.0',engine_installed:true,engine_running:true,engine_manifest:{kernel_version:'0.3.11',package_version:'0.3.11-beta.9-hardened',api_version:'v1'},state:{river:{producer:null,references:[]},instances:{},speakers:[]},inputs:{inputs:[]},speakers:{speakers:[]},routes:{routes:[]},test_tone:{phase:'idle'},background:{service_phase:'running'},environment:{platform:'android',android_api:36}};
  window.__nativeCalls=[];window.Air2MiNative={requestAsync(id,route,method,raw){window.__nativeCalls.push(route);let body=state;
   if(route==='/api/engine/v1/license')body={activated:false,can_clear:false,max_instances:2,max_outputs:2,device_id:'AVD1-1234-1234-1234-1234-1234-1234-1234-1234',reason:'not_activated'};
   if(route==='/api/plus/offers')body={schema_version:1,app_id:'air2mi',product_id:'plus',enabled:true,purchasable:true,currency:'CNY',price_minor:1990,title:'Air2Mi Plus',description:'测试商品',button_text:'立即购买',purchase_url:'https://example.com/buy'};
   if(route==='/api/background')body={service_phase:'running',notifications_allowed:true,battery_unrestricted:false};
   if(route==='/api/logs?q=&level=all')body={text:'INFO Test fixture only'};
   if(route==='/api/discovery')body={discovered:0,added:0,updated:0,skipped:0,bind:'192.0.2.2'};
   setTimeout(()=>window.__air2miReply(id,JSON.stringify({status:200,body})),route==='/api/discovery'?150:5);
  },pickEngine(){window.__nativeCalls.push('pickEngine')},copyText(){},exportDiagnostics(){window.__nativeCalls.push('export')},backgroundSettings(mode){window.__nativeCalls.push(mode)}};
 });
 await page.goto(`http://127.0.0.1:${server.address().port}`);await page.getByRole('heading',{level:1}).waitFor();
 for(const [route,name] of [['/','overview'],['/inputs','inputs'],['/speakers','speakers'],['/river','river'],['/plus','plus'],['/settings','settings']]){
  await page.evaluate(route=>{location.hash='#'+route},route);await page.waitForTimeout(150);
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error('horizontal overflow '+width+' '+name);
  if(name==='settings'){await page.getByRole('button',{name:'电池优化设置',exact:true}).click();if(!await page.evaluate(()=>window.__nativeCalls.includes('battery')))throw Error('settings adapter missing');}
  if(name==='plus'){await page.getByText('AVD1-', {exact:false}).first().waitFor();if((await page.locator('body').innerText()).includes('AMD1-'))throw Error('real device code in UI');}
  await page.screenshot({path:path.join(report,`${name}-${width}.png`),fullPage:true});results.push({width,page:name,overflow:false});
 }
 await page.close();
}await fs.writeFile(path.join(report,'browser-tests.json'),JSON.stringify({fixture:true,hardware_audio:false,pages:results},null,2));console.log('Android UI: 6 pages × 2 sizes passed; platform settings and AVD1 checked');}
finally{await browser.close();server.close();}
