// Isolated direct MCP only: no Vault, key or control-plane connection.
import fs from 'node:fs';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import { performance } from 'node:perf_hooks';
import readline from 'node:readline';

const index = process.argv.indexOf('--runtime-root');
if (index < 0 || !process.argv[index + 1]) throw new Error('--runtime-root must name the fixed 0.2.4 runtime code');
const runtimeRoot = path.resolve(process.argv[index + 1]);
const node = path.join(runtimeRoot, 'node', 'node.exe');
const entry = path.join(runtimeRoot, 'node_modules', '@wonderwhy-er', 'desktop-commander', 'dist', 'index.js');
const fixtureRoot = path.resolve('.local', 'activity-benchmark');
fs.mkdirSync(fixtureRoot, { recursive: true });
const seconds = 10, count = 100;
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const stats = pid => JSON.parse(execFileSync('powershell.exe', ['-NoProfile', '-Command', `Get-Process -Id ${Number(pid)} | Select-Object CPU,WorkingSet64,PrivateMemorySize64 | ConvertTo-Json -Compress`], { windowsHide: true, encoding: 'utf8' }));
const samples = [];

for (const mode of ['before-0.2.4', 'after-details-off', 'after-details-on']) {
  const fixture = fs.mkdtempSync(path.join(fixtureRoot, mode + '-'));
  const profile = path.join(fixture, 'profile');
  const config = path.join(profile, '.claude-server-commander');
  fs.mkdirSync(config, { recursive: true }); fs.mkdirSync(path.join(profile, 'Temp'), { recursive: true });
  fs.writeFileSync(path.join(config, 'config.json'), JSON.stringify({ allowedDirectories: [fixture], telemetryEnabled: false, defaultShell: 'powershell.exe', fileReadLineLimit: 100, fileWriteLineLimit: 50 }));
  const stdio = mode === 'before-0.2.4' ? path.join(runtimeRoot, 'stdio.mjs') : path.join(fixture, 'stdio.mjs');
  if (mode !== 'before-0.2.4') {
    for (const name of ['stdio.mjs', 'activity.mjs']) fs.copyFileSync(path.resolve('internal/managed/assets', name), path.join(fixture, name));
  }
  const args = [stdio, '--no-onboarding']; if (mode === 'after-details-on') args.push('--activity-details');
  const child = spawn(node, args, { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'], env: { SystemRoot: 'C:\\Windows', WINDIR: 'C:\\Windows', PATH: '', XPOTATO_MCP_PROFILE: profile, XPOTATO_MCP_COMMANDER_ENTRY: entry } });
  const pending = new Map(); let serial = 0, stderrBytes = 0;
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; });
  const lines = readline.createInterface({ input: child.stdout, crlfDelay: Infinity });
  lines.on('line', line => {
    if (line.length > 1024*1024) throw new Error('synthetic response exceeded bound');
    const response = JSON.parse(line); const job = pending.get(response.id);
    if (job) { pending.delete(response.id); clearTimeout(job.timer); response.error ? job.reject(new Error('synthetic request failed')) : job.resolve(response.result); }
  });
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = ++serial; const timer = setTimeout(() => reject(new Error('synthetic request timeout')), 5000);
    pending.set(id, { resolve, reject, timer }); child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  });
  try {
    await request('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'isolated-benchmark', version: '1' } });
    child.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n');
    await request('tools/list', {});
    await pause(1000);
    const idleBegin = stats(child.pid); const idleAt = performance.now(); await pause(seconds*1000); const idleEnd = stats(child.pid); const idleSeconds = (performance.now()-idleAt)/1000;
    const loadBegin = stats(child.pid); const loadAt = performance.now(); const latency = []; const bytesBefore = stderrBytes;
    for (let n = 0; n < count; n++) {
      const start = performance.now(); await request('tools/call', { name: 'get_config', arguments: {} }); latency.push(performance.now()-start);
      await pause(Math.max(0, loadAt + (n+1)*seconds*1000/count - performance.now()));
    }
    const loadEnd = stats(child.pid); const loadSeconds = (performance.now()-loadAt)/1000; latency.sort((a,b)=>a-b);
    samples.push({ mode, nodeVersion: '24.14.0', commanderVersion: '0.2.52', completed: count, idleSeconds, idleCpuSeconds: idleEnd.CPU-idleBegin.CPU, idleRssBegin: idleBegin.WorkingSet64, idleRssEnd: idleEnd.WorkingSet64, idlePrivateEnd: idleEnd.PrivateMemorySize64, loadSeconds, loadCpuSeconds: loadEnd.CPU-loadBegin.CPU, loadRssEnd: loadEnd.WorkingSet64, loadPrivateEnd: loadEnd.PrivateMemorySize64, latencyMedianMs: latency[Math.floor(count*0.5)], latencyP95Ms: latency[Math.floor(count*0.95)], stderrBytes: stderrBytes-bytesBefore });
  } finally {
    lines.close(); child.stdin.end(); child.kill();
    for (const job of pending.values()) clearTimeout(job.timer);
  }
}
const result = { note: 'Three separate direct stdio Node/DC fixtures; 10s idle and 100 synthetic get_config calls paced over 10s each; no live connection or actual credentials; RSS snapshots not peaks; console rendering cost excluded.', samples };
fs.writeFileSync(path.join(fixtureRoot, 'results.json'), JSON.stringify(result, null, 2)+'\n');
console.log(JSON.stringify(result, null, 2));
