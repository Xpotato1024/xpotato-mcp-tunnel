'use strict';
// Packaged local assets only. No fetch, remote pages, analytics, clipboard,
// browser storage, shell command or persisted credentials in the webview.
const $ = id => document.getElementById(id);
const requests = new Map();
let connected = false, supported = false, closing = false, currentPrompt = null;
let maintenance = null, connection = null, lastStatus = '', pollBusy = false;
const invoke = (name, args = {}) => window.__TAURI__.core.invoke(name, args);
const names = {setup:'初期設定',start:'接続',update:'更新',repair:'修復',doctor:'簡易診断',deep:'全依存診断',rollback:'直前の版に戻す',credentials:'キーの更新','uninstall-entrypoints':'削除の準備'};
function status(message) { $('status').textContent = message; lastStatus = message; }
function activity(message) {
  const lines = $('activity').textContent.split('\n').filter(Boolean);
  lines.push(message); $('activity').textContent = lines.slice(-40).join('\n');
}
function clearPrompt() {
  $('answer').value = ''; currentPrompt = null;
  $('interaction').hidden = true; $('choice-buttons').replaceChildren();
  $('prompt-details').textContent = '';
}
function render() {
  document.querySelectorAll('[data-action]').forEach(button => {
    button.disabled = !connected || !supported || closing || (button.dataset.action === 'start' ? Boolean(connection) : Boolean(maintenance));
  });
  $('stop').disabled = !connection || closing;
  $('cancel').disabled = !maintenance || closing;
  $('deep').disabled = !connected || !supported || Boolean(maintenance) || closing;
  $('home').disabled = Boolean(maintenance || connection) || closing;
  $('device').disabled = Boolean(maintenance || connection) || closing;
  $('connection-status').textContent = connection ? 'このアプリから実行中' : '停止中';
  $('maintenance-status').textContent = maintenance ? names[maintenance.action] + '中' : '待機中';
  $('busy-progress').hidden = !maintenance || Boolean(currentPrompt);
  if (closing) { clearPrompt(); status('このアプリの操作・接続を終了しています。既存の他プロセスは停止しません。'); }
}
async function operate(action) {
  if (!connected || !supported || closing || (action === 'start' ? connection : maintenance)) return;
  const request = {id:crypto.randomUUID(), action, started:Date.now()};
  requests.set(request.id,request);
  if (action === 'start') connection = request; else maintenance = request;
  $('diagnosis').hidden = true; status(names[action] + 'を開始しています…'); render();
  try { await invoke('operate', {id:request.id, action, home:$('home').value.trim(), device:$('device').value.trim()}); }
  catch { finish(request.id, 'failed', '操作を開始できません。アプリの接続状態を確認してください。'); }
}
function finish(id, kind, message) {
  const request = requests.get(id);
  if (!request) return;
  requests.delete(id);
  if (maintenance?.id === id) maintenance = null;
  if (connection?.id === id) connection = null;
  if (currentPrompt?.id === id) clearPrompt();
  const label = kind === 'complete' ? '完了' : kind === 'cancelled' ? '中断' : kind === 'timeout' ? '期限切れ' : '失敗';
  status(names[request.action] + '：' + label + '。' + message);
  activity(names[request.action] + '：' + label); render();
}
async function answer(value) {
  const prompt = currentPrompt;
  if (!prompt) return;
  $('answer').value = ''; clearPrompt();
  try { await invoke('reply', {id:prompt.id, prompt:prompt.prompt, value}); }
  catch { status('入力を送信できませんでした。操作を中断して再試行してください。'); }
  finally { value = ''; }
  render();
}
function prompt(event) {
  clearPrompt(); currentPrompt = event;
  $('interaction').hidden = false;
  $('prompt-title').textContent = event.kind === 'secret' ? 'API キーの入力' : event.kind === 'consent' ? '保存内容への同意' : '設定の確認';
  $('prompt-label').textContent = event.message;
  $('prompt-details').textContent = event.details || '';
  const choice = Boolean(event.choices?.length);
  $('input-row').hidden = choice; $('answer-label').hidden = choice;
  $('secret-note').hidden = event.kind !== 'secret';
  $('answer').type = event.kind === 'secret' ? 'password' : 'text';
  $('answer').autocomplete = event.kind === 'secret' ? 'new-password' : 'off';
  $('choose-folder').hidden = event.kind === 'secret';
  for (const option of event.choices || []) {
    const button = document.createElement('button'); button.type = 'button'; button.textContent = option.label;
    button.addEventListener('click', () => answer(option.value)); $('choice-buttons').append(button);
  }
  (choice ? $('choice-buttons').firstElementChild : $('answer'))?.focus();
  status('表示内容を確認して入力してください。'); render();
  $('interaction').scrollIntoView({block:'nearest'});
}
function receive(event) {
  if (event.type === 'hello') {
    supported = Boolean(event.supported); $('home').value = event.home || '';
    $('version').textContent = event.version + ' · ' + event.platform;
    $('platform-note').textContent = supported ? 'この端末だけに適用します。設定変更とキー保存は内容を確認してから進めます。' : 'この OS/CPU の runtime は準備中です。GUI の起動だけで、設定や資格情報は変更していません。';
    $('uninstall-note').textContent = event.platform.startsWith('windows') ? 'Windows の「インストールされているアプリ」から削除します。' : event.platform.startsWith('darwin') ? 'アプリを終了してから Applications 内の Xpotato Tunnel.app を削除します。' : 'OS のパッケージ管理画面から Xpotato Tunnel を削除します。';
    status('操作を選んでください。'); return;
  }
  if (event.type === 'disconnected') {
    connected = false; clearPrompt(); requests.clear(); maintenance = connection = null; status(event.message); return;
  }
  if (!requests.has(event.id)) return;
  if (event.type === 'prompt') { prompt(event); return; }
  if (event.type === 'progress' || event.type === 'working') {
    activity(event.message);
    if (!currentPrompt) status(event.message);
    return;
  }
  if (['complete','cancelled','timeout','failed','rejected'].includes(event.type)) {
    if (event.home) $('home').value = event.home;
    if (event.result) { $('diagnosis').textContent = JSON.stringify(event.result,null,2); $('diagnosis').hidden = false; }
    finish(event.id,event.type,event.message || '');
  }
}
document.querySelectorAll('[data-action]').forEach(button => button.addEventListener('click',()=>operate(button.dataset.action)));
$('answer-form').addEventListener('submit',event => { event.preventDefault(); answer($('answer').value); });
$('choose-folder').addEventListener('click',async()=> {
  try { const path = await invoke('pick_folder'); if (path && currentPrompt?.kind === 'text') $('answer').value = path; }
  catch { status('フォルダーを選択できませんでした。絶対パスを入力できます。'); }
});
async function cancel(request) {
  if (!request) return;
  if (currentPrompt?.id === request.id) clearPrompt();
  status('中断を要求しました。所有する操作の終了を待っています…');
  try { await invoke('cancel',{id:request.id}); } catch { status('中断要求を送信できません。アプリの状態を確認してください。'); }
}
$('stop').addEventListener('click',()=>cancel(connection));
$('cancel').addEventListener('click',()=>cancel(maintenance));
$('quit').addEventListener('click',async()=>{ closing = true; render(); try { await invoke('request_close'); } catch { closing = false; render(); status('終了を要求できませんでした。'); } });
$('deep').addEventListener('click',()=>$('deep-dialog').showModal());
$('deep-no').addEventListener('click',()=>$('deep-dialog').close());
$('deep-yes').addEventListener('click',()=>{ $('deep-dialog').close(); operate('deep'); });
setInterval(()=> {
  const active = maintenance || connection;
  $('elapsed').textContent = active ? '経過 ' + Math.floor((Date.now()-active.started)/1000) + ' 秒' : '';
},1000);
setInterval(async()=> {
  if (pollBusy || !window.__TAURI__) return;
  pollBusy = true;
  try { const result = await invoke('poll'); connected = result.connected; closing = result.closing; result.events.forEach(receive); render(); }
  catch { connected = false; render(); if (lastStatus !== 'アプリとの通信に失敗しました。') status('アプリとの通信に失敗しました。'); }
  finally { pollBusy = false; }
},200);
render();
