import test from 'node:test';
import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { argumentSummary, boundedWriter, instrumentTools } from '../internal/managed/assets/activity.mjs';

function observed(details = false, write, color = false) {
  class Server { setRequestHandler(schema, handler) { this.handler = handler; } }
  let output = '';
  instrumentTools(Server, { details, color, write: write ?? (s => { output += s; }) });
  return { server: new Server(), schema: { shape: { method: { value: 'tools/call' } } }, output: () => output };
}

test('default lifecycle preserves synthetic result and never displays arguments/results', async () => {
  const f = observed();
  const result = { content: [{ text: 'PRIVATE_RESULT_MARKER' }] };
  f.server.setRequestHandler(f.schema, async request => result);
  assert.equal(await f.server.handler({ params: { name: 'read_file', arguments: { path: 'PRIVATE_ARGUMENT_MARKER' } } }), result);
  assert.match(f.output(), /\[RUN\] \[MCP #1\] read_file 処理開始/);
  assert.match(f.output(), /\[OK\] \[MCP #1\] read_file 応答成功 \(\d+ms\)/);
  assert.doesNotMatch(f.output(), /PRIVATE_/);
});

test('isError and thrown failures are distinguished without exposing error content', async () => {
  const f = observed();
  f.server.setRequestHandler(f.schema, async () => ({ isError: true, content: [{ text: 'PRIVATE_ERROR_MARKER' }] }));
  await f.server.handler({ params: { name: 'start_process' } });
  assert.match(f.output(), /失敗応答/);
  const error = new Error('PRIVATE_THROWN_MARKER');
  f.server.setRequestHandler(f.schema, async () => { throw error; });
  await assert.rejects(f.server.handler({ params: { name: 'start_process' } }), e => e === error);
  assert.match(f.output(), /\[ERR\].*処理失敗 \(\d+ms\)/);
  assert.doesNotMatch(f.output(), /PRIVATE_/);
});

test('explicit detail is bounded, multiline, conservative and control safe', () => {
  const summary = argumentSummary({ command: 'echo first\r\necho second\x1b[31m\u202e', path: 'C:/Work/file.txt', content: 'FILE_BODY_MARKER', API_KEY: 'KEY_MARKER', token: 'TOKEN_MARKER', auth: 'AUTH_MARKER', password: 'PASSWORD_MARKER', env: { ANY: 'ENV_MARKER' }, other: 'OTHER_MARKER' });
  assert.match(summary, /command: echo first\n  echo second\\u001b\[31m\\u202e/);
  assert.doesNotMatch(summary, /MARKER|[\x1b\u202e]/);
  for (const command of ['curl --password PASSWORD_MARKER', 'curl -H "Authorization: Bearer TOKEN_MARKER"', 'ANY=ENV_MARKER echo go', 'echo $UNLABELED', 'echo sk-12345678901234567890123456789012345']) {
    assert.doesNotMatch(argumentSummary({ command }), /MARKER|UNLABELED|sk-123/);
  }
  assert.ok(argumentSummary({ command: 'x'.repeat(100_000) }).length < 4096);
});

test('detail withholds URL userinfo and attached credential switches', () => {
  for (const command of ['curl -ualice:purple123', 'curl -u"alice:purple123"', 'mysql -ppurple123', 'curl --proxy-user alice:purple123', 'net use X: //host/share /user:alice purple123', 'curl https://alice:purple123@example.invalid', 'ssh ssh://alice@example.invalid']) {
    assert.doesNotMatch(argumentSummary({ command }), /purple123|alice/);
  }
  assert.doesNotMatch(argumentSummary({ path: 'https://alice:purple123@example.invalid' }), /purple123|alice/);
});

test('slow activity sink drops bounded output and resumes only after drain', async () => {
  const stream = new EventEmitter();
  stream.writableLength = 0;
  let writes = 0;
  stream.write = text => { writes++; stream.writableLength += Buffer.byteLength(text); return false; };
  const write = boundedWriter(stream, 256);
  assert.equal(write('x'.repeat(257)), false);
  assert.equal(writes, 0);
  const f = observed(true, write);
  const result = { ok: true };
  f.server.setRequestHandler(f.schema, async () => result);
  for (let i = 0; i < 1000; i++) assert.equal(await f.server.handler({ params: { name: 'get_config', arguments: {} } }), result);
  assert.equal(writes, 1);
  assert.ok(stream.writableLength <= 256);
  assert.equal(stream.listenerCount('drain'), 1);
  stream.writableLength = 0;
  stream.emit('drain');
  write('resumed');
  assert.equal(writes, 2);
});

test('asynchronous display errors disable output and preserve handler results', async () => {
  const stream = new EventEmitter();
  stream.writableLength = 0;
  let writes = 0;
  stream.write = () => { writes++; return true; };
  const f = observed(true, boundedWriter(stream));
  const result = { ok: true };
  f.server.setRequestHandler(f.schema, async () => result);
  assert.equal(await f.server.handler({ params: { name: 'get_config' } }), result);
  await new Promise(resolve => setImmediate(() => { stream.emit('error', new Error('synthetic EPIPE')); resolve(); }));
  const before = writes;
  assert.equal(await f.server.handler({ params: { name: 'get_config' } }), result);
  assert.equal(writes, before);
});

test('observer failures and pathological details cannot change a request result', async () => {
  const f = observed(true, () => { throw new Error('broken display'); });
  f.server.setRequestHandler(f.schema, async () => ({ ok: true }));
  const args = { paths: [] }; args.paths.push(args.paths);
  assert.deepEqual(await f.server.handler({ params: { name: 'read_file', arguments: args } }), { ok: true });
});

test('unknown tool names and other protocol methods disclose no payload', async () => {
  const f = observed();
  f.server.setRequestHandler(f.schema, async () => ({}));
  await f.server.handler({ params: { name: 'sk-PRIVATE_TOOL_MARKER' } });
  assert.doesNotMatch(f.output(), /PRIVATE_TOOL/);
  const other = observed();
  other.server.setRequestHandler({ shape: { method: { value: 'initialize' } } }, async () => ({}));
  await other.server.handler({ params: {} });
  assert.equal(other.output(), '');
});

test('activity stays silent without requests and plain unless launcher enables color', async () => {
  const plain = observed();
  plain.server.setRequestHandler(plain.schema, async () => ({}));
  await new Promise(resolve => setTimeout(resolve, 20));
  assert.equal(plain.output(), '');
  await plain.server.handler({ params: { name: 'list_directory' } });
  assert.doesNotMatch(plain.output(), /\x1b|[\u{1f300}-\u{1faff}]/u);
  const colored = observed(false, undefined, true);
  colored.server.setRequestHandler(colored.schema, async () => ({ isError: true }));
  await colored.server.handler({ params: { name: 'list_directory' } });
  assert.match(colored.output(), /\x1b\[36m\[RUN\]\x1b\[0m/);
  assert.match(colored.output(), /\x1b\[31m\[ERR\]\x1b\[0m.*失敗応答/);
  assert.equal(plain.output().split('\n').filter(Boolean).length, 2);
});
