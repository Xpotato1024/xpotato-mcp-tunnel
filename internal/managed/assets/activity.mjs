import { performance } from 'node:perf_hooks';

const privateName = /key|token|auth|passw|secret|credential|cookie|header|env|content|text|data|result|output|replacement|old.?string|new.?string/i;
const visibleNames = new Set(['command', 'path', 'paths', 'sessionId', 'pid', 'timeout_ms', 'offset', 'length', 'searchType', 'maxResults']);
const toolNames = new Set(['get_config', 'set_config_value', 'read_file', 'read_multiple_files', 'write_file', 'write_pdf', 'create_directory', 'list_directory', 'move_file', 'start_search', 'get_more_search_results', 'stop_search', 'list_searches', 'get_file_info', 'edit_block', 'start_process', 'read_process_output', 'interact_with_process', 'force_terminate', 'list_sessions', 'list_processes', 'kill_process', 'get_usage_stats', 'get_recent_tool_calls', 'give_feedback_to_desktop_commander', 'get_prompts']);

// A slow display pipe must not accumulate activity or delay tool responses.
export function boundedWriter(stream, limit = 64 * 1024) {
  let blocked = false;
  let disabled = false;
  stream.on('error', () => { disabled = true; });
  return text => {
    if (disabled || blocked || stream.writableLength + Buffer.byteLength(text, 'utf8') > limit) return false;
    try {
      if (!stream.write(text)) {
        blocked = true;
        stream.once('drain', () => { blocked = false; });
      }
    } catch {
      disabled = true;
      return false;
    }
    return true;
  };
}
const writeActivity = boundedWriter(process.stderr);

function safeText(value) {
  return value.replace(/\r\n?/g, '\n').replace(/[\x00-\x09\x0b-\x1f\x7f-\x9f\u2028-\u202e\u2066-\u2069]/g,
    c => `\\u${c.charCodeAt(0).toString(16).padStart(4, '0')}`);
}

// Deliberately conservative: suspicious commands/strings are withheld whole.
// Never expand variables, inspect the environment, read files or show results.
function visibleValue(value, depth = 0) {
  if (depth > 2) return '[非表示]';
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  if (typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) return value.slice(0, 8).map(v => visibleValue(v, depth + 1)).join(', ');
  if (typeof value !== 'string') return '[非表示]';
  if (value.length > 2048 || /key|token|auth|passw|secret|credential|bearer|cookie|sk-[a-z0-9]|[a-z0-9_+/=-]{32,}|(?:^|[\s;])(?:-u|-h|-p|--(?:proxy-)?user|--header|-headers|-body|\/user:)|[a-z][a-z0-9+.-]*:\/\/[^\s/@]+(?::[^\s/@]*)?@|(?:^|[\s;])\$?[a-z_][a-z0-9_]*\s*=|\$[a-z_{]|%[a-z_][a-z0-9_]*%/i.test(value)) return '[秘密を含む可能性があるため非表示]';
  return safeText(value);
}

export function argumentSummary(args) {
  if (!args || typeof args !== 'object' || Array.isArray(args)) return '[引数の値は非表示]';
  const lines = [];
  for (const key in args) {
    if (!Object.hasOwn(args, key)) continue;
    if (lines.length === 16) break;
    const visible = !privateName.test(key) && visibleNames.has(key);
    const name = visible ? key : '[非表示の引数]';
    const display = visible ? visibleValue(args[key]) : '[非表示]';
    lines.push(`${name}: ${display}`.split('\n').map(line => `  ${line}`).join('\n'));
  }
  return lines.join('\n').slice(0, 4096);
}

export function activityLine(tag, message, color = false) {
  const colors = { RUN: '36', OK: '32', ERR: '31', INFO: '36' };
  const marker = `[${tag}]`;
  return `${color ? `\x1b[${colors[tag] ?? '36'}m${marker}\x1b[0m` : marker} ${message}\n`;
}

export function instrumentTools(Server, { details = false, color = false, write = writeActivity } = {}) {
  const register = Server.prototype.setRequestHandler;
  let sequence = 0;
  const emit = s => { try { write(s); } catch { /* display cannot alter a tool response */ } };
  Server.prototype.setRequestHandler = function(schema, handler, ...rest) {
    if (schema?.shape?.method?.value !== 'tools/call') return register.call(this, schema, handler, ...rest);
    return register.call(this, schema, async function(request, ...extra) {
      const started = performance.now();
      const id = ++sequence;
      const rawName = request?.params?.name;
      const name = toolNames.has(rawName) ? rawName : '[ツール名非表示]';
      emit(activityLine('RUN', `[MCP #${id}] ${name} 処理開始`, color));
      if (details) {
        let summary = '[引数の値は非表示]';
        try { summary = argumentSummary(request?.params?.arguments); } catch { /* observer is best effort */ }
        emit(activityLine('INFO', `[MCP #${id}] 明示指定の引数詳細（本文・結果は非表示）\n${summary}`, color));
      }
      try {
        const result = await handler.call(this, request, ...extra);
        emit(activityLine(result?.isError ? 'ERR' : 'OK', `[MCP #${id}] ${name} ${result?.isError ? '失敗応答' : '応答成功'} (${Math.round(performance.now() - started)}ms)`, color));
        return result;
      } catch (error) {
        emit(activityLine('ERR', `[MCP #${id}] ${name} 処理失敗 (${Math.round(performance.now() - started)}ms)`, color));
        throw error;
      }
    }, ...rest);
  };
}
