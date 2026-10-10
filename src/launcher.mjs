import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import readline from 'node:readline';
import { spawn } from 'node:child_process';
import { bundledManifest, ensureRelease, verifyRelease, validateManifest, releaseId, atomicJson, exists, fail, hash } from './install.mjs';
import { normalIdentity, validSid, assertOwner } from './identity.mjs';

const fullyQualifiedPath = value => typeof value === 'string' && path.win32.isAbsolute(value) && path.win32.parse(value).root.length > 1;

export const help = `xpotato-mcp-tunnel (Windows x64 / Node.js 22+)
  setup --device NAME --tunnel-id ID --allow ABSOLUTE_DIR [--allow ABSOLUTE_DIR]
  start --device NAME
  doctor --device NAME
  update --device NAME
  各コマンドで --home ABSOLUTE_DIR を指定可能。既定はユーザーの AppData/Local/xpotato-mcp-tunnel。
  start はキーをマスク入力し、終了するまでフォアグラウンドで動作します。`;

export function parse(args) {
  const [command, ...rest] = args;
  if (!command || command === '--help' || command === 'help') return { command: 'help' };
  if (!['setup', 'start', 'doctor', 'update'].includes(command)) throw fail(help);
  const options = { command, allow: [] };
  for (let i = 0; i < rest.length; i += 2) {
    const key = rest[i]; const value = rest[i + 1];
    if (!['--device', '--home', '--tunnel-id', '--allow'].includes(key) || !value || value.startsWith('--')) throw fail('オプションを確認してください。\n' + help);
    if (key === '--allow') options.allow.push(value);
    else {
      const property = { '--device': 'device', '--home': 'home', '--tunnel-id': 'tunnelId' }[key];
      if (options[property]) throw fail(`重複したオプション: ${key}`);
      options[property] = value;
    }
  }
  if (!/^[a-z0-9][a-z0-9_-]{0,63}$/.test(options.device ?? '')) throw fail('--device は小文字英数字、_、- で指定してください。');
  if (options.home && !fullyQualifiedPath(options.home)) throw fail('--home はドライブまたは UNC share を含む Windows の絶対パスで指定してください。');
  if (command !== 'setup' && (options.tunnelId || options.allow.length)) throw fail('--tunnel-id/--allow は setup のオプションです。');
  return options;
}

export function locations(options) {
  const home = options.home ?? path.join(os.homedir(), 'AppData', 'Local', 'xpotato-mcp-tunnel');
  const directory = path.join(home, 'devices', options.device);
  const profile = path.join(directory, 'profile');
  return { home, directory, profile, device: path.join(directory, 'device.json'),
    config: path.join(profile, '.claude-server-commander', 'config.json'), lock: path.join(directory, 'start.lock') };
}

async function readDevice(p) {
  let device;
  try { device = JSON.parse(await fs.readFile(p.device, 'utf8')); }
  catch { throw fail('端末設定がありません、または JSON が壊れています。setup/doctor で確認してください。'); }
  if (device.schemaVersion !== 2 || !validSid(device.ownerSid) || !/^tunnel_[a-zA-Z0-9_-]+$/.test(device.tunnelId ?? '') ||
      !Array.isArray(device.approvedDirectories) || !device.approvedDirectories.length ||
      device.approvedDirectories.some(root => !fullyQualifiedPath(root))) {
    throw fail('所有者と許可ルートを保存した schema 2 profile が必要です。旧 profile は変更せず、新しい --device で setup してください。');
  }
  validateManifest(device.runtime ?? {});
  return device;
}

async function readConfig(p) {
  let config;
  try { config = JSON.parse(await fs.readFile(p.config, 'utf8')); }
  catch { throw fail('Desktop Commander 設定がありません、または JSON が壊れています。'); }
  if (!Array.isArray(config.allowedDirectories) || !config.allowedDirectories.length ||
      config.allowedDirectories.some(root => !fullyQualifiedPath(root))) {
    throw fail('allowedDirectories に明示的な絶対パスが必要です。空配列は使用できません。');
  }
  return config;
}

function assertApprovedRoots(config, device) {
  for (const root of config.allowedDirectories) {
    const approved = device.approvedDirectories.some(parent => {
      const relative = path.win32.relative(parent, root);
      return !path.win32.isAbsolute(relative) && relative !== '..' && !relative.startsWith('..\\');
    });
    if (!approved) throw fail('allowedDirectories が setup 時の許可ルートを超えています。新しい --device で明示的に setup してください。');
  }
}

async function ownedDevice(p, deps) {
  const identity = await normalIdentity(deps.identityReader);
  const device = await readDevice(p);
  assertOwner(device, identity);
  return device;
}

export function quoteMcpPath(value) {
  if (/[\r\n'\0]/.test(value)) throw fail('Node/runtime パスのシングルクォート・改行は非対応です。別の保存先を指定してください。');
  return `'${value.replaceAll('\\', '/')}'`;
}

export function tunnelArgs(p, device, release, run, node = process.execPath) {
  const command = `${quoteMcpPath(node)} ${quoteMcpPath(path.join(release, 'stdio.mjs'))} --no-onboarding`;
  return ['run', '--control-plane.tunnel-id', device.tunnelId,
    '--control-plane.api-key', 'env:CONTROL_PLANE_API_KEY', '--control-plane.max-inflight', '1',
    '--control-plane.poll-channel', 'main', '--mcp.command', command, '--mcp.max-concurrent-requests', '1',
    '--mcp.stdio-send-initialized-notification', '--health.listen-addr', '127.0.0.1:0',
    '--health.url-file', path.join(run, 'health-url.txt'), '--log.level', 'info', '--log.format', 'json',
    '--log.file', path.join(run, 'tunnel-info.log'), '--log.http-raw-unsafe=false', '--pid.file', path.join(run, 'tunnel.pid')];
}

export async function maskedKey(input = process.stdin, output = process.stdout) {
  if (!input.isTTY || !output.isTTY) throw fail('キー入力には対話ターミナルが必要です。通常ユーザーの PowerShell で start を実行してください。');
  readline.emitKeypressEvents(input);
  const previousRaw = input.isRaw;
  output.write('Runtime API key (masked; local only): ');
  input.setRawMode(true); input.resume();
  let key = '';
  try {
    return await new Promise((resolve, reject) => {
      const listener = (text, stroke = {}) => {
        if (stroke.ctrl && stroke.name === 'c') { input.off('keypress', listener); reject(fail('キー入力を中止しました。')); }
        else if (stroke.name === 'return' || stroke.name === 'enter') {
          input.off('keypress', listener);
          key ? resolve(key) : reject(fail('キーが空です。'));
        } else if (stroke.name === 'backspace') {
          if (key.length) { key = key.slice(0, -1); output.write('\b \b'); }
        } else if (text && !stroke.ctrl && !stroke.meta && !/[\x00-\x1f\x7f]/.test(text)) {
          key += text; output.write('*'.repeat(text.length));
        }
      };
      input.on('keypress', listener);
    });
  } finally { key = ''; input.setRawMode(Boolean(previousRaw)); input.pause(); output.write('\n'); }
}

async function withMaintenance(p, action) {
  await fs.mkdir(p.directory, { recursive: true });
  const file = path.join(p.directory, 'maintenance.lock');
  let lock;
  try { lock = await fs.open(file, 'wx'); }
  catch (error) {
    if (error.code === 'EEXIST') throw fail('この端末の setup/update が処理中です。doctor で lock を確認してください。');
    throw error;
  }
  try { return await action(); }
  finally { await lock.close(); await fs.unlink(file); }
}

export async function setup(options, deps = {}) {
  const p = locations(options);
  const identity = await normalIdentity(deps.identityReader);
  // Reject unowned/elevated state before taking a writable maintenance lock.
  if (await exists(p.device)) assertOwner(await readDevice(p), identity);
  else if (await exists(p.config)) throw fail('所有者未確認の既存 profile は引き継ぎません。別の --device で setup してください。');
  return withMaintenance(p, async () => {
    const old = await exists(p.device) ? await readDevice(p) : undefined;
    const previousConfig = await exists(p.config) ? await readConfig(p) : undefined;
    if (old) { assertOwner(old, identity); if (previousConfig) assertApprovedRoots(previousConfig, old); }
    if (old && options.tunnelId && old.tunnelId !== options.tunnelId) throw fail('既存端末の tunnelId は上書きしません。別の --device を指定してください。');
    if (previousConfig && options.allow.length && JSON.stringify(previousConfig.allowedDirectories) !== JSON.stringify(options.allow)) {
      throw fail('既存 allowedDirectories は上書きしません。保存済み config.json を本人が編集してください。');
    }
    const tunnelId = old?.tunnelId ?? options.tunnelId;
    if (!/^tunnel_[a-zA-Z0-9_-]+$/.test(tunnelId ?? '')) throw fail('新しい端末には --tunnel-id tunnel_... が必要です。');
    if (!previousConfig && (!options.allow.length || options.allow.some(root => !fullyQualifiedPath(root)))) throw fail('新しい端末にはドライブまたは UNC share を含む --allow の絶対パスが必要です。');
    for (const root of previousConfig?.allowedDirectories ?? options.allow) {
      const stat = await fs.stat(root).catch(() => null);
      if (!stat?.isDirectory()) throw fail('--allow の保存先ディレクトリが存在しません。先に本人が作成してください。');
    }
    const manifest = old?.runtime ?? deps.manifest ?? bundledManifest;
    await (deps.ensureRelease ?? ensureRelease)(p.home, manifest, deps.install);
    for (const sub of ['AppData/Roaming', 'AppData/Local', 'Temp']) await fs.mkdir(path.join(p.profile, sub), { recursive: true });
    if (!previousConfig) {
      await atomicJson(p.config, {
        allowedDirectories: options.allow,
        blockedCommands: ['shutdown', 'reboot', 'format', 'diskpart', 'netsh', 'runas', 'takeown', 'reg', 'sc'],
        defaultShell: 'powershell.exe', fileReadLineLimit: 100, fileWriteLineLimit: 50, telemetryEnabled: false
      });
    }
    if (!old) await atomicJson(p.device, { schemaVersion: 2, ownerSid: identity.sid,
      approvedDirectories: [...options.allow], tunnelId, runtime: manifest });
    return { config: p.config, preserved: Boolean(old || previousConfig), runtime: releaseId(manifest) };
  });
}

export async function update(options, deps = {}) {
  const p = locations(options);
  const prepared = await ownedDevice(p, deps);
  return withMaintenance(p, async () => {
    const device = await readDevice(p);
    assertOwner(device, { sid: prepared.ownerSid });
    assertApprovedRoots(await readConfig(p), device);
    const manifest = deps.manifest ?? bundledManifest;
    await (deps.ensureRelease ?? ensureRelease)(p.home, manifest, deps.install);
    await atomicJson(p.device, { ...device, runtime: manifest });
    return { runtime: releaseId(manifest), applies: 'next-start' };
  });
}

export async function start(options, deps = {}) {
  const p = locations(options);
  const device = await ownedDevice(p, deps);
  assertApprovedRoots(await readConfig(p), device);
  const release = path.join(p.home, 'releases', releaseId(device.runtime));
  await (deps.verifyRelease ?? verifyRelease)(release, device.runtime);
  // Validate quoting before the prompt. Lock collisions never prompt or kill a process.
  quoteMcpPath(deps.node ?? process.execPath); quoteMcpPath(path.join(release, 'stdio.mjs'));
  let lock;
  try { lock = await fs.open(p.lock, 'wx'); }
  catch (error) {
    if (error.code === 'EEXIST') throw fail('この端末は起動中、または前回の start.lock が残っています。doctor で確認してください。自動終了・再起動は行いません。');
    throw error;
  }
  let credential = '';
  let tunnelLock;
  const tunnelLockFile = path.join(p.home, 'tunnel-locks', `${hash(device.tunnelId)}.lock`);
  let child;
  let interrupted = false;
  const signal = () => { interrupted = true; child?.kill(); };
  try {
    await lock.writeFile(JSON.stringify({ launcherPid: process.pid }));
    await fs.mkdir(path.dirname(tunnelLockFile), { recursive: true });
    try { tunnelLock = await fs.open(tunnelLockFile, 'wx'); }
    catch (error) {
      if (error.code === 'EEXIST') throw fail('同じ保存先の別 profile でこの Tunnel が起動中、または lock が残っています。並列起動せず doctor で確認してください。');
      throw error;
    }
    await tunnelLock.writeFile(JSON.stringify({ launcherPid: process.pid }));
    credential = await (deps.credentialReader ?? maskedKey)();
    if (typeof credential !== 'string' || !credential.length || /[\r\n\0]/.test(credential)) throw fail('キー入力の形式を確認してください。');
    const run = await fs.mkdtemp(path.join(p.directory, 'run-'));
    const env = { ...process.env, CONTROL_PLANE_API_KEY: credential,
      XPOTATO_MCP_PROFILE: p.profile, XPOTATO_MCP_COMMANDER_ENTRY: path.join(release, 'node_modules', '@wonderwhy-er', 'desktop-commander', 'dist', 'index.js') };
    try {
      child = (deps.spawn ?? spawn)(path.join(release, 'tunnel-client-runtime.exe'), tunnelArgs(p, device, release, run, deps.node),
        { shell: false, stdio: 'inherit', windowsHide: true, env });
    } finally { credential = ''; delete env.CONTROL_PLANE_API_KEY; }
    process.on('SIGINT', signal); process.on('SIGTERM', signal);
    return await new Promise((resolve, reject) => {
      child.once('error', () => reject(fail('Tunnel runtime を起動できませんでした。doctor で確認してください。')));
      child.once('close', code => resolve(interrupted ? 130 : code ?? 1));
    });
  } finally {
    credential = '';
    process.off('SIGINT', signal); process.off('SIGTERM', signal);
    if (tunnelLock) { await tunnelLock.close(); await fs.unlink(tunnelLockFile); }
    await lock.close(); await fs.unlink(p.lock);
  }
}

export async function doctor(options, deps = {}) {
  const p = locations(options);
  const device = await ownedDevice(p, deps);
  const config = await readConfig(p);
  assertApprovedRoots(config, device);
  const release = path.join(p.home, 'releases', releaseId(device.runtime));
  await (deps.verifyRelease ?? verifyRelease)(release, device.runtime);
  const locks = {};
  for (const [name, file] of [['start', p.lock], ['tunnel', path.join(p.home, 'tunnel-locks', `${hash(device.tunnelId)}.lock`)],
    ['maintenance', path.join(p.directory, 'maintenance.lock')], ['install', path.join(p.home, 'releases', 'install.lock')]]) {
    locks[name] = await exists(file);
  }
  return { node: process.versions.node, runtime: releaseId(device.runtime), allowedDirectoryCount: config.allowedDirectories.length,
    locks, integrity: 'ok', connection: 'not-probed', credentials: 'not-read' };
}

export async function main(args, deps = {}) {
  const options = parse(args);
  const output = deps.output ?? console.log;
  if (options.command === 'help') { output(help); return 0; }
  if ((deps.platform ?? process.platform) !== 'win32' || (deps.arch ?? process.arch) !== 'x64') throw fail('この版は Windows x64 専用です。');
  if (Number(process.versions.node.split('.')[0]) < 22) throw fail('Node.js 22 以上が必要です。');
  if (options.command === 'start') return start(options, deps);
  const result = await ({ setup, update, doctor })[options.command](options, deps);
  output(JSON.stringify(result, null, 2));
  return 0;
}
