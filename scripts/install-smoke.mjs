// Opt-in real install smoke. No control-plane connection or API key.
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { spawn } from 'node:child_process';
import { ensureRelease, bundledManifest } from '../src/install.mjs';
import { setup, locations } from '../src/launcher.mjs';

if (process.platform !== 'win32' || process.arch !== 'x64' || !process.argv[2]) {
  console.error('Windows x64: npm run smoke:install -- ABSOLUTE_PATH_TO_APPROVED_RUNTIME_ZIP');
  process.exit(1);
}
const archive = path.resolve(process.argv[2]);
const home = await fs.mkdtemp(path.join(os.tmpdir(), 'xpotato real smoke '));
const started = Date.now();
let child;
try {
  const release = await ensureRelease(home, bundledManifest, { download: (_url, destination) => fs.copyFile(archive, destination) });
  const root = path.join(home, 'Allowed fixture'); await fs.mkdir(root);
  const options = { home, device: 'offline-smoke', tunnelId: 'tunnel_fixture', allow: [root] };
  await setup(options);
  const p = locations(options);
  const version = await new Promise((resolve, reject) => {
    const processChild = spawn(path.join(release, 'tunnel-client-runtime.exe'), ['--version'], { shell: false, stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
    let out = ''; processChild.stdout.on('data', b => { out += b; });
    processChild.once('error', reject); processChild.once('close', code => code === 0 ? resolve(out.trim()) : reject(Error('runtime version failed')));
  });
  child = spawn(process.execPath, [path.join(release, 'stdio.mjs'), '--no-onboarding'], {
    shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, XPOTATO_MCP_PROFILE: p.profile,
      XPOTATO_MCP_COMMANDER_ENTRY: path.join(release, 'node_modules/@wonderwhy-er/desktop-commander/dist/index.js') }
  });
  const toolCount = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => { child.kill(); reject(Error('MCP fixture timeout')); }, 60000);
    let out = ''; let completed = false;
    child.stdout.on('data', bytes => {
      out += bytes;
      for (;;) {
        const index = out.indexOf('\n'); if (index < 0) break;
        const line = out.slice(0, index); out = out.slice(index + 1);
        let message; try { message = JSON.parse(line); } catch { continue; }
        if (message.id === 1 && message.result) {
          child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
          child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} }) + '\n');
        }
        if (message.id === 2 && Array.isArray(message.result?.tools)) {
          completed = true; clearTimeout(timer); resolve(message.result.tools.length);
        }
      }
    });
    child.stderr.resume();
    child.once('error', () => { clearTimeout(timer); reject(Error('MCP fixture spawn failed')); });
    child.once('close', () => { clearTimeout(timer); if (!completed) reject(Error('MCP fixture closed before response')); });
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: {
      protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'offline-fixture', version: '1.0.0' }
    } }) + '\n');
  });
  if (toolCount < 1) throw Error('No tools');
  console.log(JSON.stringify({ archive: 'approved-local-copy', runtimeVersion: version,
    commanderVersion: bundledManifest.commanderVersion, toolCount, elapsedMs: Date.now() - started,
    profile: 'isolated', controlPlaneConnection: 'not-started', credentials: 'not-used' }));
} finally {
  if (child && child.exitCode === null) {
    await new Promise(resolve => { child.once('close', resolve); child.kill(); });
  }
  // Only the owned mkdtemp fixture is removed.
  await fs.rm(home, { recursive: true, force: true });
}
