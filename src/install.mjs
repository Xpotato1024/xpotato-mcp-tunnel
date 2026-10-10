import fs from 'node:fs/promises';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { suppressStartupChrome, assertStartupChromeSuppressed } from './commander-compat.mjs';

export const bundledManifest = JSON.parse(await fs.readFile(new URL('../runtime/manifest.json', import.meta.url), 'utf8'));
export const hash = bytes => createHash('sha256').update(bytes).digest('hex');
export const fail = message => Object.assign(new Error(message), { safeMessage: message });
export const exists = async file => fs.access(file).then(() => true, () => false);

export function validateManifest(m) {
  if (m.schemaVersion !== 1 || !Number.isSafeInteger(m.stdioVersion) || m.stdioVersion < 2 || m.chromeStartup !== 'suppressed' || m.platform !== 'win32-x64' ||
      !/^\d+\.\d+\.\d+$/.test(m.tunnelVersion) || !/^\d+\.\d+\.\d+$/.test(m.commanderVersion) ||
      !/^[a-f0-9]{64}$/.test(m.archiveSha256) || !/^[a-f0-9]{64}$/.test(m.executableSha256) ||
      m.url !== `https://github.com/openai/tunnel-client/releases/download/v${m.tunnelVersion}/tunnel-client-runtime-v${m.tunnelVersion}-windows-amd64.zip`) {
    throw fail('runtime manifest の形式・公式 URL・SHA256 を確認してください。');
  }
  return m;
}

export const releaseId = m => `${m.tunnelVersion}-dc-${m.commanderVersion}-${hash(JSON.stringify(m)).slice(0, 16)}`;

export async function renameStrict(source, destination, rename = fs.rename) {
  try { await rename(source, destination); }
  catch (error) {
    if (error.code === 'EXDEV') throw fail('別ドライブへの rename は非対応です。元ファイルを保持しました。同一ドライブで実行してください。');
    throw error;
  }
}

export async function atomicJson(file, value, rename = fs.rename) {
  await fs.mkdir(path.dirname(file), { recursive: true });
  const temp = `${file}.${process.pid}.${Math.random().toString(16).slice(2)}.tmp`;
  await fs.writeFile(temp, `${JSON.stringify(value, null, 2)}\n`, { flag: 'wx' });
  try { await renameStrict(temp, file, rename); }
  finally { await fs.unlink(temp).catch(() => {}); }
}

export async function runNative(executable, args, options = {}) {
  await new Promise((resolve, reject) => {
    const child = spawn(executable, args, { shell: false, stdio: 'inherit', ...options });
    child.once('error', () => reject(fail('依存 runtime の実行に失敗しました。Node.js/npm と接続を確認してください。')));
    child.once('close', code => code === 0 ? resolve() : reject(fail(`依存 runtime の準備が終了コード ${code} で失敗しました。`)));
  });
}

async function download(url, destination) {
  const response = await fetch(url, { signal: AbortSignal.timeout(120000) });
  if (!response.ok) throw fail(`公式 runtime の取得に失敗しました (HTTP ${response.status})。`);
  const bytes = Buffer.from(await response.arrayBuffer());
  await fs.writeFile(destination, bytes, { flag: 'wx' });
}

async function extract(archive, destination) {
  // No interpolated command text or npm.cmd invocation through a shell.
  await runNative('powershell.exe', ['-NoProfile', '-NonInteractive', '-File',
    fileURLToPath(new URL('./expand-runtime.ps1', import.meta.url)), archive, destination]);
}

async function installCommander(directory, version) {
  const npmCli = path.join(path.dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js');
  if (!await exists(npmCli)) throw fail('Node.js と同じ場所の npm-cli.js が見つかりません。公式 Node.js の npm を確認してください。');
  await fs.writeFile(path.join(directory, 'package.json'), JSON.stringify({ private: true, name: 'xpotato-device-runtime', version: '1.0.0' }));
  await runNative(process.execPath, [npmCli, 'install', '--save-exact', '--ignore-scripts', '--no-audit', '--no-fund',
    `@wonderwhy-er/desktop-commander@${version}`], { cwd: directory });
}

export async function verifyRelease(directory, manifest) {
  const receipt = JSON.parse(await fs.readFile(path.join(directory, 'receipt.json'), 'utf8'));
  if (JSON.stringify(receipt.manifest) !== JSON.stringify(manifest)) throw fail('runtime receipt と manifest が一致しません。');
  const required = ['tunnel-client-runtime.exe', 'stdio.mjs', 'package-lock.json',
    'node_modules/@wonderwhy-er/desktop-commander/dist/index.js'];
  if (JSON.stringify(Object.keys(receipt.files).sort()) !== JSON.stringify(required.sort())) throw fail('runtime receipt の対象ファイルが不足しています。');
  for (const relative of required) {
    if (hash(await fs.readFile(path.join(directory, relative))) !== receipt.files[relative]) throw fail('runtime ファイルの SHA256 が一致しません。');
  }
  if (receipt.files['tunnel-client-runtime.exe'] !== manifest.executableSha256) throw fail('公式 runtime の SHA256 が一致しません。');
  assertStartupChromeSuppressed(await fs.readFile(path.join(directory, 'node_modules/@wonderwhy-er/desktop-commander/dist/index.js'), 'utf8'));
  return receipt;
}

export async function ensureRelease(home, manifest, deps = {}) {
  validateManifest(manifest);
  const releases = path.join(home, 'releases');
  const destination = path.join(releases, releaseId(manifest));
  if (await exists(destination)) { await verifyRelease(destination, manifest); return destination; }
  await fs.mkdir(releases, { recursive: true });
  const lockPath = path.join(releases, 'install.lock');
  let lock;
  try { lock = await fs.open(lockPath, 'wx'); }
  catch (error) {
    if (error.code === 'EEXIST') throw fail('別の setup/update が処理中です。install.lock を doctor で確認してください。');
    throw error;
  }
  let stage;
  try {
    stage = await fs.mkdtemp(path.join(releases, '.stage-'));
    const archive = path.join(stage, 'runtime.zip');
    await (deps.download ?? download)(manifest.url, archive);
    if (hash(await fs.readFile(archive)) !== manifest.archiveSha256) throw fail('取得した ZIP の SHA256 が一致しません。既存設定は変更していません。');
    await (deps.extract ?? extract)(archive, stage);
    await fs.unlink(archive);
    // The official runtime ZIP currently puts this binary at its root.
    if (hash(await fs.readFile(path.join(stage, 'tunnel-client-runtime.exe'))) !== manifest.executableSha256) throw fail('展開した公式 runtime の SHA256 が一致しません。');
    await (deps.installCommander ?? installCommander)(stage, manifest.commanderVersion);
    const commanderEntry = path.join(stage, 'node_modules', '@wonderwhy-er', 'desktop-commander', 'dist', 'index.js');
    await fs.writeFile(commanderEntry, suppressStartupChrome(await fs.readFile(commanderEntry, 'utf8')));
    await fs.copyFile(fileURLToPath(new URL('./stdio.mjs', import.meta.url)), path.join(stage, 'stdio.mjs'));
    const files = {};
    for (const relative of ['tunnel-client-runtime.exe', 'stdio.mjs', 'package-lock.json',
      'node_modules/@wonderwhy-er/desktop-commander/dist/index.js']) {
      files[relative] = hash(await fs.readFile(path.join(stage, relative)));
    }
    await atomicJson(path.join(stage, 'receipt.json'), { manifest, files });
    await verifyRelease(stage, manifest);
    await renameStrict(stage, destination);
    return destination;
  } finally {
    // stage is an owned mkdtemp child of releases, never an option supplied by the user.
    try { if (stage) await fs.rm(stage, { recursive: true, force: true }); }
    finally { await lock.close(); await fs.unlink(lockPath); }
  }
}
