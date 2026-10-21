// Hosted Windows integration only. No Setup, Vault, keys, runtime or user data.
// Test-process CDP follows https://playwright.dev/docs/webview2 ; it is never
// enabled by the packaged application. No shell, taskkill or process search.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const {once}=require('node:events');
const fs=require('node:fs/promises');
const path=require('node:path');
const net=require('node:net');
const {chromium}=require(process.env.XPOTATO_PLAYWRIGHT_MODULE || 'playwright');

test('native WebView2, Tauri permissions and actual Go bridge hello/close', {timeout:45000},async()=>{
  assert.equal(process.env.GITHUB_ACTIONS,'true');
  assert.equal(process.env.RUNNER_ENVIRONMENT,'github-hosted');
  assert.equal(process.platform,'win32');
  const executable=path.resolve('gui/src-tauri/target/release/xpotato-tunnel-app.exe');
  const profile=await fs.mkdtemp(path.join(process.env.RUNNER_TEMP,'xpotato-native-webview-'));
  const reservation=net.createServer();
  reservation.listen(0,'127.0.0.1');await once(reservation,'listening');
  const port=reservation.address().port;await new Promise(resolve=>reservation.close(resolve));
  const child=spawn(executable,[],{shell:false,windowsHide:true,stdio:'ignore',env:{...process.env,
    WEBVIEW2_USER_DATA_FOLDER:profile,
    WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS:`--remote-debugging-port=${port} --remote-debugging-address=127.0.0.1`}});
  const exited=new Promise((resolve,reject)=>{child.once('exit',(code)=>resolve(code));child.once('error',reject);});
  let browser;
  try {
    const deadline=Date.now()+25000;
    while(Date.now()<deadline && child.exitCode===null){
      try {browser=await chromium.connectOverCDP(`http://127.0.0.1:${port}`,{timeout:1000});break;}
      catch {await new Promise(resolve=>setTimeout(resolve,150));}
    }
    assert.ok(browser,'isolated native WebView did not become available');
    const context=browser.contexts()[0];
    let page=context.pages()[0];
    if(!page)page=await context.waitForEvent('page',{timeout:5000});
    await page.waitForFunction(()=>document.getElementById('version')?.textContent.includes('windows/amd64'),null,{timeout:10000});
    assert.match(await page.locator('#version').innerText(),/0\.2\.11/);
    assert.equal(await page.locator('[data-action=setup]').isEnabled(),true);
    // A cancellation with no owned matching ID is an intentional no-op.
    await page.evaluate(()=>window.__TAURI__.core.invoke('cancel',{id:'fixture-no-such-owned-operation'}));
    await assert.rejects(page.evaluate(()=>window.__TAURI__.core.invoke('plugin:fs|read_dir',{path:'fixture-not-an-actual-path'})));
    await page.screenshot({path:'dist/gui-native-ready.png'});
    await page.locator('#quit').click();
    assert.equal(await Promise.race([exited,new Promise((_,reject)=>setTimeout(()=>reject(new Error('owned native app did not drain')),5000))]),0);
  } finally {
    if(browser)await browser.close().catch(()=>{});
    if(child.exitCode===null){child.kill();await exited.catch(()=>{});} // This exact fixture child only; Go sees pipe EOF.
    // Leave the dedicated profile to ephemeral runner disposal; never recursive delete.
  }
});
