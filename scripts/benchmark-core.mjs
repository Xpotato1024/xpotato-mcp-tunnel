// Run the same synthetic-vault/mock-runtime test helper on both source versions.
import fs from 'node:fs';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import readline from 'node:readline';
const root = path.resolve('.local/activity-benchmark');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const stats = pid => JSON.parse(execFileSync('powershell.exe', ['-NoProfile', '-Command', `Get-Process -Id ${Number(pid)} | Select-Object CPU,WorkingSet64,PrivateMemorySize64 | ConvertTo-Json -Compress`], { windowsHide: true, encoding: 'utf8' }));
const samples = [];
for (const mode of ['before', 'after']) {
  const child = spawn(path.join(root, `core-${mode}.test.exe`), ['-test.run=^TestCoreIdlePerformanceFixture$'], { windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], env: { SystemRoot: 'C:\\Windows', WINDIR: 'C:\\Windows', PATH: '', TEMP: root, TMP: root, XPOTATO_PERF_CORE: '1' } });
  child.stderr.on('data', () => {});
  const lines = readline.createInterface({ input: child.stdout });
  const exited = new Promise(resolve => child.once('exit', (code) => resolve(code)));
  try {
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('synthetic core readiness timeout')), 10000);
      lines.on('line', line => { if (line === 'PERF_READY') { clearTimeout(timeout); resolve(); } });
    });
    const begin = stats(child.pid); await pause(10000); const end = stats(child.pid);
    samples.push({ mode, fixture: 'actual core.Start and managed.Run with synthetic Vault and local mock Tunnel; test executable, not release RSS', idleSeconds: 10, cpuSeconds: end.CPU - begin.CPU, rssBegin: begin.WorkingSet64, rssEnd: end.WorkingSet64, privateEnd: end.PrivateMemorySize64 });
    if (await exited !== 0) throw new Error('synthetic core did not clean up normally');
  } finally { lines.close(); if (child.exitCode === null) execFileSync('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' }); }
}
const result = { note: 'Two source versions; identical 10s idle fixture. Official Tunnel/Node tool-load measured separately. No real owner state, Vault or key. RSS snapshots include test executable overhead.', samples };
fs.writeFileSync(path.join(root, 'core-results.json'), JSON.stringify(result, null, 2)+'\n');
console.log(JSON.stringify(result, null, 2));
