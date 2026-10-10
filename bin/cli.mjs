#!/usr/bin/env node
import { main } from '../src/launcher.mjs';

try {
  process.exitCode = await main(process.argv.slice(2));
} catch (error) {
  // Child output is inherited; never dump environment, credential or spawn arguments.
  console.error(error.safeMessage ?? '処理に失敗しました。設定と runtime を doctor で確認してください。');
  process.exitCode = 1;
}
