// Fixed official Tunnel + Node/DC; only a local synthetic control plane.
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import { spawn, execFileSync } from 'node:child_process';
import { performance } from 'node:perf_hooks';
const rootAt = process.argv.indexOf('--runtime-root');
if (rootAt < 0) throw new Error('--runtime-root required');
const root = path.resolve(process.argv[rootAt+1]);
const fixtureRoot = path.resolve('.local/activity-benchmark'); fs.mkdirSync(fixtureRoot,{recursive:true});
const pause = ms => new Promise(r=>setTimeout(r,ms));
const stats = (pid, children=false) => JSON.parse(execFileSync('powershell.exe',['-NoProfile','-Command',children ? `@(Get-CimInstance Win32_Process | Where-Object { $_.ParentProcessId -eq ${Number(pid)} } | Select-Object ProcessId,Name) | ConvertTo-Json -Compress` : `Get-Process -Id ${Number(pid)} | Select-Object CPU,WorkingSet64,PrivateMemorySize64 | ConvertTo-Json -Compress`],{windowsHide:true,encoding:'utf8'}) || '[]');
const samples=[];
for(const mode of ['before-0.2.4','after-details-off']) {
 const fixture=fs.mkdtempSync(path.join(fixtureRoot,'tunnel-'+mode+'-'));
 const profile=path.join(fixture,'profile'); fs.mkdirSync(path.join(profile,'.claude-server-commander'),{recursive:true});fs.mkdirSync(path.join(profile,'Temp'),{recursive:true});
 fs.writeFileSync(path.join(profile,'.claude-server-commander','config.json'),JSON.stringify({allowedDirectories:[fixture],telemetryEnabled:false,defaultShell:'powershell.exe'}));
 let stdio=path.join(root,'stdio.mjs');
 if(mode!=='before-0.2.4'){for(const name of ['stdio.mjs','activity.mjs'])fs.copyFileSync(path.resolve('internal/managed/assets',name),path.join(fixture,name));stdio=path.join(fixture,'stdio.mjs');}
 const pending=new Map();const queue=[];let poll=null,serial=0,stderrBytes=0,syntheticDiagnostic='';
 const flush=()=>{if(poll&&queue.length){clearTimeout(poll.timer);poll.res.setHeader('Content-Type','application/json');poll.res.end(JSON.stringify({commands:[queue.shift()]}));poll=null;}};
 const server=http.createServer((req,res)=>{
  if(req.url.includes('/poll')){poll={res,timer:setTimeout(()=>{res.writeHead(204);res.end();if(poll?.res===res)poll=null;},250)};flush();return;}
  if(req.url.includes('/response')){let body='';req.on('data',b=>{body+=b;if(body.length>1024*1024)req.destroy();});req.on('end',()=>{const data=JSON.parse(body);const job=pending.get(data.request_id);if(job){pending.delete(data.request_id);clearTimeout(job.timer);data.resp_json?.error?job.reject(new Error('synthetic MCP error')):job.resolve(data);}res.writeHead(200);res.end('{}');});return;}
  res.setHeader('Content-Type','application/json');res.end('{}');
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const request=(method,params)=>new Promise((resolve,reject)=>{const requestId=`synthetic-${++serial}`;const timer=setTimeout(()=>reject(new Error('synthetic tunnel timeout')),8000);pending.set(requestId,{resolve,reject,timer});queue.push({request_id:requestId,shard_token:'synthetic-shard',command_type:'jsonrpc',channel:'main',created_at:new Date().toISOString(),headers:{'Content-Type':['application/json']},jsonrpc:{jsonrpc:'2.0',id:serial,method,params}});flush();});
 const command=`'${path.join(root,'node','node.exe').replaceAll('\\','/')}' '${stdio.replaceAll('\\','/')}' --no-onboarding`;
 const child=spawn(path.join(root,'tunnel-client-runtime.exe'),['run','--control-plane.base-url',`http://127.0.0.1:${server.address().port}`,'--control-plane.url-path','','--control-plane.tunnel-id','tunnel_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','--control-plane.api-key','env:CONTROL_PLANE_API_KEY','--control-plane.max-inflight','1','--control-plane.poll-channel','main','--mcp.command',command,'--mcp.max-concurrent-requests','1','--mcp.stdio-send-initialized-notification','--health.listen-addr','127.0.0.1:0','--log.level','info','--log.format','','--log.file',''],{cwd:fixture,windowsHide:true,stdio:['pipe','pipe','pipe'],env:{SystemRoot:'C:\\Windows',WINDIR:'C:\\Windows',PATH:'',CONTROL_PLANE_API_KEY:'synthetic-benchmark-key-not-real',XPOTATO_MCP_PROFILE:profile,XPOTATO_MCP_COMMANDER_ENTRY:path.join(root,'node_modules','@wonderwhy-er','desktop-commander','dist','index.js')}});
 child.stderr.on('data',b=>{stderrBytes+=b.length;syntheticDiagnostic=(syntheticDiagnostic+b.toString()).slice(-8192);});child.stdout.on('data',()=>{});
 try {
  await request('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'synthetic-benchmark',version:'1'}});
  await request('tools/list',{});
  await pause(1000);
  const children=stats(child.pid,true);const nodeChild=(Array.isArray(children)?children:[children]).find(p=>p.Name==='node.exe');if(!nodeChild)throw new Error('isolated Node child missing');
  const first={tunnel:stats(child.pid),node:stats(nodeChild.ProcessId)};await pause(10000);const idle={tunnel:stats(child.pid),node:stats(nodeChild.ProcessId)};
  const latencies=[],began=performance.now(),bytesBefore=stderrBytes;
  for(let n=0;n<100;n++){const start=performance.now();await request('tools/call',{name:'get_config',arguments:{}});latencies.push(performance.now()-start);await pause(Math.max(0,began+(n+1)*100-performance.now()));}
  const load={tunnel:stats(child.pid),node:stats(nodeChild.ProcessId)};latencies.sort((a,b)=>a-b);
  samples.push({mode,tunnelVersion:'0.0.16',nodeVersion:'24.14.0',commanderVersion:'0.2.52',completed:100,first,idle,load,loadSeconds:(performance.now()-began)/1000,latencyMedianMs:latencies[50],latencyP95Ms:latencies[95],stderrBytes:stderrBytes-bytesBefore});
 } catch (error) {
  console.error('Synthetic fixture diagnostic:',syntheticDiagnostic.replaceAll('synthetic-benchmark-key-not-real','[synthetic-key-redacted]').replace(/[\x00-\x08\x0b-\x1f\x7f]/g,'?'));
  throw error;
 } finally {
  for(const job of pending.values())clearTimeout(job.timer);
  if(child.exitCode===null)execFileSync('taskkill.exe',['/PID',String(child.pid),'/T','/F'],{windowsHide:true,stdio:'ignore'});
  if(poll){clearTimeout(poll.timer);poll.res.end();}server.closeAllConnections();await new Promise(r=>server.close(r));
 }
}
const result={note:'Official Tunnel runtime 0.0.16 against only 127.0.0.1 synthetic poll/response; same fixed Node/DC. Separate processes, 10s idle then 100 requests paced over 10s. No owner setup, Vault, actual keys or live service.',samples};fs.writeFileSync(path.join(fixtureRoot,'tunnel-results.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result,null,2));
