import {chromium} from '../web/node_modules/playwright-core/index.mjs';
const [base,key,out]=process.argv.slice(2);const browser=await chromium.launch({headless:true});
const pages=[['','overview'],['inputs','inputs'],['speakers','speakers'],['river','river'],['settings','settings']];
try{
 for(const [width,height] of [[1440,810],[390,910],[768,1024]]){
  const page=await browser.newPage({viewport:{width,height}});const errors=[];page.on('pageerror',e=>errors.push(e.message));await page.addInitScript(key=>sessionStorage.setItem('air2mi-dev-key',key),key);
  await page.goto(base+'/');await page.getByRole('heading',{name:'播放概览'}).waitFor();
  for(const[route,name]of pages){await page.goto(base+'/#/'+route);await page.locator('h1').waitFor();await page.waitForTimeout(150);if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('horizontal overflow '+width+' '+route);await page.screenshot({path:`${out}/${name}-${width}.png`,fullPage:true})}
  if(width===1440){
   await page.goto(base+'/#/inputs');await page.getByRole('button',{name:'添加接收器',exact:true}).first().click();await page.getByLabel('名称',{exact:true}).waitFor();if(await page.getByLabel('标识',{exact:true}).count()||await page.getByLabel('端口',{exact:true}).count())throw Error('technical receiver fields exposed');await page.getByRole('button',{name:'取消',exact:true}).click();
   await page.goto(base+'/#/speakers');await page.getByRole('button',{name:'局域网发现',exact:true}).click();await page.getByText('正在搜索妙播音箱…',{exact:true}).waitFor();if(!await page.locator('progress').isVisible())throw Error('missing progress');await page.getByText(/搜索完成/).first().waitFor({timeout:60000});if(await page.getByRole('button',{name:'导入',exact:true}).count())throw Error('manual import required');
   const tone=page.getByRole('button',{name:'播放测试音',exact:true}).first();if(await tone.count()){let started=false;const observe=request=>{if(request.method()==='POST'&&request.url().endsWith('/test-tone'))started=true};page.on('request',observe);await tone.click();const volume=page.getByRole('slider',{name:'测试音量'});await volume.fill('35');if(await page.locator('.volume-choice output').innerText()!=='35%')throw Error('test volume choice missing');await page.getByRole('button',{name:'取消',exact:true}).click();if(started)throw Error('test started before confirmation');page.off('request',observe)}
  }
  if(errors.length)throw Error(errors.join('\n'));await page.close();
 }
 console.log('Real Host UI: five pages, three viewports, receiver form, discovery progress and auto-import passed');
}finally{await browser.close()}
