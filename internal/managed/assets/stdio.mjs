import path from 'node:path';
import os from 'node:os';
import fs from 'node:fs';
import { pathToFileURL } from 'node:url';
import { instrumentTools } from './activity.mjs';
import { offerSource } from './licensing.mjs';

const sourceInfo = JSON.parse(process.env.XPOTATO_MCP_SOURCE_INFO || 'null');
if (!sourceInfo || sourceInfo.license !== 'AGPL-3.0-only') throw new Error('Source information is missing');

const profile = process.env.XPOTATO_MCP_PROFILE;
const entry = process.env.XPOTATO_MCP_COMMANDER_ENTRY;
if (!profile || !entry || !fs.existsSync(path.join(profile, '.claude-server-commander', 'config.json'))) {
  throw new Error('Prepared device configuration is missing');
}
process.env.HOME = process.env.USERPROFILE = profile;
process.env.APPDATA = path.join(profile, 'AppData', 'Roaming');
process.env.LOCALAPPDATA = path.join(profile, 'AppData', 'Local');
process.env.TEMP = process.env.TMP = process.env.TMPDIR = path.join(profile, 'Temp');
process.env.XDG_CONFIG_HOME = path.join(profile, '.config');
process.env.XDG_CACHE_HOME = path.join(profile, '.cache');
process.env.XDG_DATA_HOME = path.join(profile, '.local', 'share');
process.env.DESKTOP_COMMANDER_DISABLE_TELEMETRY = '1';
process.env.DC_FLAG_URL = 'http://127.0.0.1:1/flags.json';
for (const name of ['DC_REMOTE_DEVICE', 'LLM01_ALLOW_CHROME_DOWNLOAD', 'CONTROL_PLANE_API_KEY',
  'OPENAI_API_KEY', 'XPOTATO_MCP_PROFILE', 'XPOTATO_MCP_COMMANDER_ENTRY', 'XPOTATO_MCP_SOURCE_INFO']) delete process.env[name];
if (os.homedir() !== profile) throw new Error('Device profile did not resolve');
if (!process.argv.includes('--no-onboarding')) process.argv.push('--no-onboarding');
// Same pinned ESM Server class that Commander imports. Observe its public
// request-handler boundary without consuming/rewriting protocol streams.
const sdk = path.join(path.dirname(entry), '..', '..', '..', '@modelcontextprotocol', 'sdk', 'dist', 'esm', 'server', 'index.js');
const { Server } = await import(pathToFileURL(sdk).href);
offerSource(Server, sourceInfo);
instrumentTools(Server, { details: process.argv.includes('--activity-details'), color: process.argv.includes('--activity-color') });
await import(pathToFileURL(entry).href);
