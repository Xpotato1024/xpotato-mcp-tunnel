// Browser fixtures only: no real backend, profiles, credentials or setup.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const {pathToFileURL}=require('node:url');
const path=require('node:path');
const fs=require('node:fs/promises');
const {chromium}=require(process.env.XPOTATO_PLAYWRIGHT_MODULE || 'playwright');

test('GUI consent, secret lifetime, independent cancellation and terminal states',async()=>{
  const browser=await chromium.launch({channel:process.env.XPOTATO_BROWSER_CHANNEL || 'chrome',headless:true,chromiumSandbox:true});
  try {
    const page=await browser.newPage({viewport:{width:1000,height:950}});
    await page.route('http://**',route=>route.abort());
    await page.route('https://**',route=>route.abort());
    await page.addInitScript(()=>{
      window.fixture={events:[{type:'hello',supported:true,version:'0.2.11',platform:'windows/amd64',home:'C:/fixture'}],calls:[]};
      window.__TAURI__={core:{invoke:async(name,args)=>{
        if(name==='poll')return {connected:true,closing:false,events:window.fixture.events.splice(0)};
        window.fixture.calls.push({name,args});
        return null;
      }}};
    });
    await page.goto(pathToFileURL(path.resolve(__dirname,'../gui/ui/index.html')).href);
    await page.locator('[data-action=setup]').waitFor();
    await page.waitForFunction(()=>!document.querySelector('[data-action=setup]').disabled);
    if(process.env.XPOTATO_GUI_SCREENSHOTS){
      await fs.mkdir(process.env.XPOTATO_GUI_SCREENSHOTS,{recursive:true});
      await page.screenshot({path:path.join(process.env.XPOTATO_GUI_SCREENSHOTS,'gui-ready.png'),fullPage:true});
    }
    await page.locator('[data-action=setup]').click();
    const setup=await page.evaluate(()=>window.fixture.calls.find(c=>c.name==='operate').args.id);
    const emit=event=>page.evaluate(event=>window.fixture.events.push(event),event);
    await emit({type:'prompt',id:setup,prompt:1,kind:'consent',message:'Store in OS credential vault?',details:'fixture storage consent',choices:[{label:'Agree',value:'yes'},{label:'Cancel',value:'no'}]});
    await page.getByRole('button',{name:'Agree',exact:true}).click();
    await emit({type:'prompt',id:setup,prompt:2,kind:'secret',message:'API key (fixture)'});
    await page.locator('#answer').waitFor({state:'visible'});
    assert.equal(await page.locator('#answer').getAttribute('type'),'password');
    await page.locator('#answer').fill('fixture-only-secret');
    await page.locator('#send-answer').click();
    assert.equal(await page.locator('#answer').inputValue(),'');
    assert.doesNotMatch(await page.locator('body').innerText(),/fixture-only-secret/);
    await emit({type:'complete',id:setup,message:'fixture done'});
    await page.waitForFunction(()=>!document.querySelector('[data-action=start]').disabled && !document.querySelector('[data-action=update]').disabled);
    await page.locator('[data-action=start]').click();
    await page.locator('[data-action=update]').click();
    const calls=await page.evaluate(()=>window.fixture.calls.filter(c=>c.name==='operate'));
    const start=calls.find(c=>c.args.action==='start').args.id;
    const update=calls.find(c=>c.args.action==='update').args.id;
    await emit({type:'progress',id:update,message:'download: 1024 / 2048 bytes; 3 files'});
    await page.waitForFunction(()=>document.getElementById('status').textContent.includes('1024 / 2048 bytes; 3 files'));
    await page.locator('#cancel').click();
    assert.equal((await page.evaluate(()=>window.fixture.calls.filter(c=>c.name==='cancel').at(-1))).args.id,update);
    assert.equal(await page.locator('#stop').isEnabled(),true);
    await emit({type:'timeout',id:update,message:'bounded fixture deadline; existing runtime preserved'});
    await page.waitForFunction(()=>document.getElementById('status').textContent.includes('bounded fixture deadline'));
    const terminal=await page.locator('#status').innerText();
    await emit({type:'progress',id:'stale-id',message:'SHOULD NOT DISPLAY'});
    await page.waitForTimeout(300);
    assert.equal(await page.locator('#status').innerText(),terminal);
    await page.locator('#stop').click();
    assert.equal((await page.evaluate(()=>window.fixture.calls.filter(c=>c.name==='cancel').at(-1))).args.id,start);
    await emit({type:'cancelled',id:start,message:'own fixture stopped'});
    await page.waitForFunction(()=>document.getElementById('stop').disabled);
    await page.locator('details.panel > summary').click();
    await page.locator('#deep').click();
    assert.equal(await page.locator('#deep-dialog').isVisible(),true);
    assert.equal((await page.evaluate(()=>window.fixture.calls.filter(c=>c.args?.action==='deep'))).length,0);
    await page.locator('#deep-no').click();
    await page.locator('[data-action=setup]').click();
    const second=await page.evaluate(()=>window.fixture.calls.filter(c=>c.name==='operate').at(-1).args.id);
    await emit({type:'prompt',id:second,prompt:3,kind:'secret',message:'fixture key'});
    await page.locator('#answer').waitFor({state:'visible'});
    await page.locator('#answer').fill('cleared-on-close');
    await page.locator('#quit').click();
    assert.equal(await page.locator('#answer').inputValue(),'');
    assert.equal((await page.evaluate(()=>window.fixture.calls.at(-1))).name,'request_close');
  } finally {await browser.close();}
});
