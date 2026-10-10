import path from 'node:path';
import os from 'node:os';
import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

const profile = process.env.XPOTATO_MCP_PROFILE;
const entry = process.env.XPOTATO_MCP_COMMANDER_ENTRY;
if (!profile || !entry || !fs.existsSync(path.join(profile, '.claude-server-commander', 'config.json'))) {
  throw new Error('Prepared device configuration is missing');
}
process.env.USERPROFILE = profile;
process.env.APPDATA = path.join(profile, 'AppData', 'Roaming');
process.env.LOCALAPPDATA = path.join(profile, 'AppData', 'Local');
process.env.TEMP = process.env.TMP = path.join(profile, 'Temp');
process.env.DESKTOP_COMMANDER_DISABLE_TELEMETRY = '1';
process.env.DC_FLAG_URL = 'http://127.0.0.1:1/flags.json';
delete process.env.DC_REMOTE_DEVICE;
delete process.env.LLM01_ALLOW_CHROME_DOWNLOAD;
delete process.env.CONTROL_PLANE_API_KEY;
delete process.env.OPENAI_API_KEY;
delete process.env.XPOTATO_MCP_PROFILE;
delete process.env.XPOTATO_MCP_COMMANDER_ENTRY;
if (os.homedir() !== profile) throw new Error('Device profile did not resolve');
if (!process.argv.includes('--no-onboarding')) process.argv.push('--no-onboarding');
await import(pathToFileURL(entry).href);
