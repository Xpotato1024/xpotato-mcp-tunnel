import path from 'node:path';
import { execFile } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { fail } from './install.mjs';

export const validSid = value => typeof value === 'string' && /^S-1-\d+(?:-\d+){1,15}$/.test(value);

export async function currentIdentity() {
  const executable = path.join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  return new Promise((resolve, reject) => {
    execFile(executable, ['-NoProfile', '-NonInteractive', '-File', fileURLToPath(new URL('./current-token.ps1', import.meta.url))],
      { windowsHide: true, timeout: 10000 }, (error, stdout) => {
        if (error) { reject(fail('起動ユーザーのトークンを確認できません。通常ユーザーの PowerShell で実行してください。')); return; }
        try { resolve(JSON.parse(stdout)); }
        catch { reject(fail('起動ユーザーのトークン確認に失敗しました。')); }
      });
  });
}

export async function normalIdentity(reader = currentIdentity) {
  const identity = await reader();
  if (!validSid(identity?.sid) || typeof identity.elevated !== 'boolean' || identity.elevated) {
    throw fail('通常ユーザーの非管理者ターミナルで実行してください。');
  }
  return identity;
}

export function assertOwner(device, identity) {
  if (device.ownerSid !== identity.sid) throw fail('この profile を setup したユーザーで実行してください。所有者の自動変更は行いません。');
}
