import {chromium} from '../web/node_modules/playwright-core/index.mjs';
const[base,key,out]=process.argv.slice(2);const browser=await chromium.launch({headless:true,executablePath:process.env.AIR2MI_BROWSER_EXECUTABLE||undefined,args:['--disable-gpu'],env:{...process.env,DBUS_SESSION_BUS_ADDRESS:'unix:path=/dev/null',DBUS_SYSTEM_BUS_ADDRESS:'unix:path=/dev/null'}});const routes=process.env.AIR2MI_PLUS_ONLY?[['plus','plus']]:[['','overview'],['inputs','inputs'],['speakers','speakers'],['river','river'],['settings','settings'],['plus','plus']];
try{for(const[width,height,device]of[[1440,1050,'desktop'],[390,844,'mobile']]){const page=await browser.newPage({viewport:{width,height},extraHTTPHeaders:{'X-Air2Mi-Screenshot-Key':key}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 // This trusted proxy does not stream SSE. Explicitly use the App's development
 // polling mode; the proxy forwards only actual installed Host API responses.
 await page.addInitScript(()=>sessionStorage.setItem('air2mi-dev-key','screenshot-proxy'));
 for(const[route,name]of routes){await page.goto(base+'/#/'+route);try{await page.locator('h1').waitFor()}catch(err){await page.screenshot({path:`${out}/failure-${device}.png`,fullPage:true,mask:[page.locator('.plus-device-code')]} );console.error('Browser errors',errors);throw err}await page.waitForTimeout(250);if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('horizontal overflow '+name+' '+device);await page.screenshot({path:`${out}/${name}-${device}.png`,fullPage:true,mask:[page.locator('.plus-device-code')]} )}
 await page.goto(base+'/#/plus');await page.getByRole('heading',{name:'Air2Mi Plus',exact:true,level:1}).waitFor();await page.getByRole('heading',{name:'已永久激活',exact:true}).waitFor();const metadata=await page.evaluate(async()=>{
 const root=location.pathname.replace(/\/$/,'');
 const headers={'X-Air2Mi-Host-Key':'screenshot-proxy'};
 const license=await fetch(root+'/api/engine/v1/license',{headers}).then(r=>r.json());
 const response=await fetch(root+'/api/plus/offers',{headers});
 return {license,offer:await response.json(),offerStatus:response.status};
});
const expected=metadata.license.device_id;
if(!/^AVD1-(?:[A-F0-9]{4}-){7}[A-F0-9]{4}$/.test(expected))throw Error('application API exposed a non-display device code');
if(await page.locator('.plus-device-code').innerText()!==expected)throw Error('display code mismatch');
if((await page.locator('body').innerText()).includes('AMD1-'))throw Error('real device id visible');
if(await page.getByRole('button',{name:'显示完整设备码',exact:true}).count())throw Error('raw id reveal still available');
await page.context().grantPermissions(['clipboard-read','clipboard-write'],{origin:new URL(base).origin});
await page.getByRole('button',{name:'复制设备识别码',exact:true}).click();
if(await page.evaluate(()=>navigator.clipboard.readText())!==expected)throw Error('clipboard did not contain display code');
if(metadata.offerStatus===200&&metadata.offer.enabled===true&&metadata.offer.purchasable===true){
 await page.locator('a.plus-buy').waitFor();
 if(await page.locator('a.plus-buy').getAttribute('href')!==metadata.offer.purchase_url)throw Error('purchase URL mismatch');
}else if(await page.locator('a.plus-buy').count())throw Error('unexpected purchase button');
const sectionOrder=await page.locator('.plus-page > section').evaluateAll(sections=>sections.map(section=>section.getAttribute('aria-label')));
if(JSON.stringify(sectionOrder)!==JSON.stringify(['商品信息','已激活状态','设备识别码','激活详情']))throw Error('Plus content order differs from PushMi');
if(metadata.license.can_clear){
 await page.getByRole('button',{name:'清除本机授权凭据',exact:true}).click();
 const clearDialog=page.getByRole('dialog',{name:'清除本机授权',exact:true});
 await clearDialog.waitFor();
 await clearDialog.getByRole('button',{name:'清除凭据',exact:true}).waitFor();
 await clearDialog.getByRole('button',{name:'保留授权',exact:true}).click();
}
if(await page.locator('#plus-cdk').isVisible())throw Error('CDK form should be in a closed dialog');
await page.getByRole('button',{name:'输入激活码',exact:true}).click();
await page.getByRole('dialog',{name:'重复激活确认'}).waitFor();
await page.getByRole('button',{name:'继续输入激活码',exact:true}).click();
const activationDialog=page.getByRole('dialog',{name:'输入激活码',exact:true});
await activationDialog.waitFor();
if(await page.locator('#plus-cdk').getAttribute('type')!=='password')throw Error('CDK input unmasked');
await page.locator('#plus-cdk').fill('invalid');
await activationDialog.getByRole('button',{name:'激活',exact:true}).click();
await activationDialog.getByText('激活码格式不正确，请检查后重新输入。',{exact:true}).waitFor();
await activationDialog.getByRole('button',{name:'清除',exact:true}).click();
if(await page.locator('#plus-cdk').inputValue()!=='')throw Error('CDK clear failed');
await activationDialog.locator('button.secondary').click();
if(await page.locator('a.plus-buy').count()){
 await page.locator('a.plus-buy').click();
 const purchaseDialog=page.getByRole('dialog',{name:'重复购买确认'});
 await purchaseDialog.waitFor();
 await purchaseDialog.getByRole('button',{name:'取消',exact:true}).click();
}
await page.screenshot({path:`${out}/plus-${device}.png`,fullPage:true,mask:[page.locator('.plus-device-code')]} );
 if(!process.env.AIR2MI_PLUS_ONLY){await page.goto(base+'/#/speakers');const test=page.getByRole('button',{name:'播放测试音',exact:true}).first();await test.waitFor();await test.click();const slider=page.getByRole('slider',{name:'测试音量'});await slider.waitFor();await slider.fill('35');if(await page.locator('.volume-choice output').innerText()!=='35%')throw Error('volume choice not reflected');await page.screenshot({path:`${out}/test-tone-${device}.png`,fullPage:true,mask:[page.locator('.plus-device-code')]} );await page.getByRole('button',{name:'取消',exact:true}).click()}
 if(errors.length)throw Error(errors.join('\n'));await page.close()}console.log('Installed-Host desktop/mobile screenshots and authorization checks passed with real API data')}finally{await browser.close()}
