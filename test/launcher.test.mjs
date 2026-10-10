import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import { fileURLToPath } from 'node:url';
import { parse, locations, quoteMcpPath, tunnelArgs, maskedKey, setup, update, start, doctor, main } from '../src/launcher.mjs';
import { bundledManifest, hash, releaseId, ensureRelease, verifyRelease, atomicJson, renameStrict } from '../src/install.mjs';

async function fixture(t) {
  const home = await fs.mkdtemp(path.join(os.tmpdir(), 'xpotato fixture '));
  t.after(() => fs.rm(home, { recursive: true, force: true }));
  const root = path.join(home, 'Allowed files 日本語'); await fs.mkdir(root);
  const options = parse(['setup', '--home', home, '--device', 'test-node', '--tunnel-id', 'tunnel_fixture', '--allow', root]);
  const p = locations(options);
  const manifest = { ...bundledManifest, executableSha256: hash('fake runtime'), archiveSha256: hash('fake archive') };
  const calls = [];
  const install = {
    download: async (url, file) => { calls.push('download'); await fs.writeFile(file, 'fake archive'); },
    extract: async (file, directory) => { calls.push('extract'); await fs.writeFile(path.join(directory, 'tunnel-client-runtime.exe'), 'fake runtime'); },
    installCommander: async (directory, version) => {
      calls.push(`npm:${version}`);
      const entry = path.join(directory, 'node_modules', '@wonderwhy-er', 'desktop-commander', 'dist');
      await fs.mkdir(entry, { recursive: true });
      await fs.writeFile(path.join(entry, 'index.js'), 'export {};');
      await fs.writeFile(path.join(directory, 'package-lock.json'), JSON.stringify({ fixture: version }));
    }
  };
  const deps = { manifest, install };
  return { home, root, options, p, manifest, calls, install, deps };
}

const readJson = async file => JSON.parse(await fs.readFile(file, 'utf8'));

test('setup creates isolated config; repeat preserves bytes, custom fields and edited limits', async t => {
  const f = await fixture(t);
  await setup(f.options, f.deps);
  const original = await readJson(f.p.config);
  await atomicJson(f.p.config, { ...original, fileReadLineLimit: 731, customOwnerValue: 'keep' });
  const before = await fs.readFile(f.p.config);
  const deviceBefore = await fs.readFile(f.p.device);
  const result = await setup({ ...f.options, tunnelId: undefined, allow: [] }, f.deps);
  assert.equal(result.preserved, true);
  assert.deepEqual(await fs.readFile(f.p.config), before);
  assert.deepEqual(await fs.readFile(f.p.device), deviceBefore);
  assert.deepEqual(f.calls, ['download', 'extract', 'npm:0.2.52']);
  await assert.rejects(setup({ ...f.options, tunnelId: 'tunnel_other' }, f.deps), /上書き/);
  await assert.rejects(setup({ ...f.options, allow: [f.home] }, f.deps), /上書き/);
});

test('setup rejects missing, nonexistent and empty allowed roots without installing', async t => {
  const f = await fixture(t);
  await assert.rejects(setup({ ...f.options, allow: [] }, f.deps), /--allow/);
  await assert.rejects(setup({ ...f.options, allow: [path.join(f.home, 'missing')] }, f.deps), /存在/);
  assert.deepEqual(f.calls, []);
  assert.equal(await fs.access(f.p.device).then(() => true, () => false), false);
});

test('different devices share immutable runtime, keep separate configs', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const other = { ...f.options, device: 'other-node', tunnelId: 'tunnel_other' };
  await setup(other, f.deps);
  assert.notEqual(locations(other).config, f.p.config);
  assert.equal((await readJson(f.p.device)).tunnelId, 'tunnel_fixture');
  assert.equal((await readJson(locations(other).device)).tunnelId, 'tunnel_other');
  assert.equal(f.calls.filter(c => c === 'download').length, 1);
});

test('update stages a new release and preserves config, owner fields, running lock and old release', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const device = await readJson(f.p.device);
  await atomicJson(f.p.device, { ...device, ownerAnnotation: 'keep' });
  const before = await fs.readFile(f.p.config);
  await fs.writeFile(f.p.lock, JSON.stringify({ launcherPid: 123 }));
  const newer = { ...f.manifest, commanderVersion: '0.2.53' };
  const result = await update(f.options, { ...f.deps, manifest: newer });
  assert.equal(result.applies, 'next-start');
  assert.deepEqual(await fs.readFile(f.p.config), before);
  assert.equal((await readJson(f.p.device)).ownerAnnotation, 'keep');
  assert.deepEqual(await readJson(f.p.lock), { launcherPid: 123 });
  await verifyRelease(path.join(f.home, 'releases', releaseId(f.manifest)), f.manifest);
  await verifyRelease(path.join(f.home, 'releases', releaseId(newer)), newer);
});

test('failed update retains selection and config; install lock and owned staging are cleaned', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const before = await fs.readFile(f.p.device); const config = await fs.readFile(f.p.config);
  const newer = { ...f.manifest, commanderVersion: '0.2.53', archiveSha256: hash('different') };
  await assert.rejects(update(f.options, { ...f.deps, manifest: newer }), /SHA256/);
  assert.deepEqual(await fs.readFile(f.p.device), before);
  assert.deepEqual(await fs.readFile(f.p.config), config);
  assert.deepEqual(await fs.readdir(path.join(f.home, 'releases')), [releaseId(f.manifest)]);
});

test('wrapper-only version update selects a separate release without changing config', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const before = await fs.readFile(f.p.config);
  const newer = { ...f.manifest, stdioVersion: 2 };
  await update(f.options, { ...f.deps, manifest: newer });
  assert.notEqual(releaseId(f.manifest), releaseId(newer));
  assert.deepEqual(await fs.readFile(f.p.config), before);
  await verifyRelease(path.join(f.home, 'releases', releaseId(newer)), newer);
});

test('doctor is read only and reports locks without key or connection access', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const before = await fs.readFile(f.p.config); const stat = await fs.stat(f.p.device);
  await fs.writeFile(f.p.lock, 'not inspected');
  const result = await doctor(f.options);
  assert.equal(result.integrity, 'ok'); assert.equal(result.locks.start, true);
  assert.equal(result.credentials, 'not-read'); assert.equal(result.connection, 'not-probed');
  assert.deepEqual(await fs.readFile(f.p.config), before);
  assert.equal((await fs.stat(f.p.device)).mtimeMs, stat.mtimeMs);
});

test('changed runtime fails verification before credential prompt', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  await fs.writeFile(path.join(f.home, 'releases', releaseId(f.manifest), 'stdio.mjs'), '// changed');
  let prompted = false;
  await assert.rejects(start(f.options, { credentialReader: async () => { prompted = true; return 'synthetic'; } }), /SHA256/);
  assert.equal(prompted, false);
});

test('start uses single quoted Windows paths and a native argv array, key only in child environment', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  let snapshot;
  const deps = { node: 'C:\\Program Files\\nodejs\\node.exe', credentialReader: async () => 'synthetic-test-key',
    spawn: (executable, args, options) => {
      snapshot = { executable, args, options: { ...options, env: { ...options.env } } };
      const child = new EventEmitter(); child.kill = () => {};
      queueMicrotask(() => child.emit('close', 7)); return child;
    } };
  assert.equal(await start(f.options, deps), 7);
  assert.equal(snapshot.options.shell, false);
  assert.equal(snapshot.options.stdio, 'inherit');
  assert.equal(snapshot.options.env.CONTROL_PLANE_API_KEY === 'synthetic-test-key', true);
  assert.equal(snapshot.args.includes('synthetic-test-key'), false);
  const command = snapshot.args[snapshot.args.indexOf('--mcp.command') + 1];
  assert.ok(command.startsWith("'C:/Program Files/nodejs/node.exe' '"));
  assert.ok(command.includes('xpotato fixture '));
  assert.ok(command.endsWith("/stdio.mjs' --no-onboarding"));
  assert.equal(await fs.access(f.p.lock).then(() => true, () => false), false);
  const runs = (await fs.readdir(f.p.directory)).filter(file => file.startsWith('run-'));
  assert.deepEqual(await fs.readdir(path.join(f.p.directory, runs[0])), []);
});

test('duplicate start fails without prompting, spawning, deleting lock or touching config', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  await fs.writeFile(f.p.lock, 'owner lock');
  const before = await fs.readFile(f.p.config);
  await assert.rejects(start(f.options, { credentialReader: () => assert.fail('prompted'), spawn: () => assert.fail('spawned') }), /起動中/);
  assert.equal(await fs.readFile(f.p.lock, 'utf8'), 'owner lock');
  assert.deepEqual(await fs.readFile(f.p.config), before);
});

test('manual restart prompts again, uses updated runtime and keeps config', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const before = await fs.readFile(f.p.config); let prompts = 0; const executables = [];
  const deps = { credentialReader: async () => { prompts++; return 'synthetic'; }, spawn: executable => {
    executables.push(executable); const child = new EventEmitter(); child.kill = () => {};
    queueMicrotask(() => child.emit('close', 0)); return child;
  } };
  await start(f.options, deps);
  const newer = { ...f.manifest, commanderVersion: '0.2.53' };
  await update(f.options, { ...f.deps, manifest: newer });
  await start(f.options, deps);
  assert.equal(prompts, 2); assert.notEqual(executables[0], executables[1]);
  assert.deepEqual(await fs.readFile(f.p.config), before);
});

test('spawn error and cancelled credential prompt remove only own lock', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  await assert.rejects(start(f.options, { credentialReader: async () => { throw Error('fixture cancellation'); } }), /fixture cancellation/);
  assert.equal(await fs.access(f.p.lock).then(() => true, () => false), false);
  await assert.rejects(start(f.options, { credentialReader: async () => 'synthetic', spawn: () => {
    const child = new EventEmitter(); queueMicrotask(() => child.emit('error', Error('fixture error'))); return child;
  } }), /起動できません/);
  assert.equal(await fs.access(f.p.lock).then(() => true, () => false), false);
});

test('cross-drive EXDEV never copies/deletes original; failed atomic write retains saved config', async t => {
  const f = await fixture(t); const file = path.join(f.home, 'config.json'); const source = path.join(f.home, 'source.txt');
  await fs.writeFile(file, 'original config'); await fs.writeFile(source, 'keep source');
  const exdev = async () => { throw Object.assign(Error('fixture'), { code: 'EXDEV' }); };
  await assert.rejects(renameStrict(source, path.join(f.home, 'destination.txt'), exdev), /元ファイル/);
  assert.equal(await fs.readFile(source, 'utf8'), 'keep source');
  await assert.rejects(atomicJson(file, { changed: true }, exdev), /元ファイル/);
  assert.equal(await fs.readFile(file, 'utf8'), 'original config');
  assert.equal((await fs.readdir(f.home)).some(name => name.endsWith('.tmp')), false);
});

test('Windows native argv preserves the entire MCP command and supports spaced executable path', async t => {
  const f = await fixture(t);
  const command = tunnelArgs(f.p, { tunnelId: 'tunnel_fixture' }, 'C:\\Runtime With Spaces', 'C:\\Run', 'C:\\Program Files\\nodejs\\node.exe');
  const probe = path.join(f.home, 'argv probe.mjs');
  await fs.writeFile(probe, 'process.stdout.write(JSON.stringify(process.argv.slice(2)));');
  const actual = await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [probe, ...command], { shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = ''; child.stdout.on('data', chunk => { out += chunk; }); child.once('error', reject);
    child.once('close', code => code === 0 ? resolve(JSON.parse(out)) : reject(Error(`probe ${code}`)));
  });
  assert.deepEqual(actual, command);
});

test('real child output is inherited and its exit code reaches start', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const code = await start(f.options, { credentialReader: async () => 'synthetic', spawn: (_exe, _args, options) =>
    spawn(process.execPath, ['-e', 'console.log("FIXTURE_CHILD_OUTPUT"); process.exit(9);'], options) });
  assert.equal(code, 9);
});

test('stdio wrapper resolves separate Windows profile and removes keys before Commander import', async t => {
  const f = await fixture(t); await setup(f.options, f.deps);
  const entry = path.join(f.home, 'commander probe.mjs');
  await fs.writeFile(entry, 'import os from "node:os"; console.log(JSON.stringify({home:os.homedir(),keyPresent:Boolean(process.env.CONTROL_PLANE_API_KEY),openaiKeyPresent:Boolean(process.env.OPENAI_API_KEY),onboardingSkipped:process.argv.includes("--no-onboarding")}));');
  const result = await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [fileURLToPath(new URL('../src/stdio.mjs', import.meta.url))],
      { shell: false, env: { ...process.env, XPOTATO_MCP_PROFILE: f.p.profile, XPOTATO_MCP_COMMANDER_ENTRY: entry,
        CONTROL_PLANE_API_KEY: 'synthetic', OPENAI_API_KEY: 'synthetic' }, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = ''; let err = ''; child.stdout.on('data', chunk => { out += chunk; }); child.stderr.on('data', chunk => { err += chunk; });
    child.once('error', reject); child.once('close', code => code === 0 ? resolve(JSON.parse(out)) : reject(Error(err)));
  });
  assert.deepEqual(result, { home: f.p.profile, keyPresent: false, openaiKeyPresent: false, onboardingSkipped: true });
});

test('masked input supports backspace, restores terminal and never echoes plaintext', async () => {
  const input = new EventEmitter(); input.isTTY = true; input.isRaw = false;
  input.setRawMode = value => { input.isRaw = value; }; input.resume = () => {}; input.pause = () => {};
  let displayed = ''; const output = { isTTY: true, write: value => { displayed += value; } };
  const promise = maskedKey(input, output);
  input.emit('keypress', 'a', { name: 'a' }); input.emit('keypress', 'b', { name: 'b' });
  input.emit('keypress', '', { name: 'backspace' }); input.emit('keypress', 'z', { name: 'z' }); input.emit('keypress', '\r', { name: 'return' });
  assert.equal(await promise === 'az', true); assert.equal(displayed.includes('az'), false);
  assert.equal(input.isRaw, false); assert.equal(input.listenerCount('keypress'), 0);
});

test('unsupported quoting, unknown/key options and unsafe device paths fail closed', async () => {
  assert.throws(() => quoteMcpPath("C:\\owner's\\node.exe"), /非対応/);
  for (const args of [['start', '--device', '../escape'], ['start', '--device', 'test', '--api-key', 'synthetic'],
    ['setup', '--device', 'test', '--home', 'relative'], ['doctor', '--device', 'test', '--allow', 'C:\\Work']]) {
    assert.throws(() => parse(args));
  }
  await assert.rejects(main(['doctor', '--device', 'test'], { platform: 'linux' }), /Windows/);
});
