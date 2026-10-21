#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::Serialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{collections::VecDeque, fs::File, io::{BufRead, BufReader, Read, Write}, process::{Command, Stdio}, sync::{Arc, Mutex, atomic::{AtomicBool, Ordering}, mpsc::{sync_channel, SyncSender, TrySendError}}};
use tauri::{Manager, State, WindowEvent};
use tauri_plugin_dialog::DialogExt;
use zeroize::Zeroize;

const LIMIT: usize = 65536;
#[derive(Default)]
struct Backend {
    input: Option<SyncSender<Vec<u8>>>,
    shutdown: Arc<AtomicBool>,
    events: VecDeque<Value>,
    closing: bool,
    connected: bool,
    alive: bool,
}
type Shared = Arc<Mutex<Backend>>;

impl Backend {
    fn send(&mut self, message: &Value) -> Result<(), String> {
        let mut frame = serde_json::to_vec(message).map_err(|_| "通信を作成できません。")?;
        if frame.len() >= LIMIT { frame.zeroize(); return Err("入力が長すぎます。".into()); }
        frame.push(b'\n');
        let Some(input) = self.input.as_ref() else { frame.zeroize(); return Err("同梱 backend に接続していません。".into()); };
        match input.try_send(frame) {
            Ok(()) => Ok(()),
            Err(TrySendError::Full(mut frame)) => { frame.zeroize(); Err("送信待ちです。少し待ってから再試行してください。".into()) },
            Err(TrySendError::Disconnected(mut frame)) => { frame.zeroize(); Err("backend との通信に失敗しました。".into()) }
        }
    }
    fn push(&mut self, event: Value) {
        // Preserve prompts and terminal states when a background window polls
        // slowly; coalesce old progress before accepting another heartbeat.
        if self.events.len() >= 256 {
            if let Some(index) = self.events.iter().position(|item| item["type"] == "progress") {
                self.events.remove(index);
            } else { return; }
        }
        self.events.push_back(event);
    }
    fn close(&mut self) {
        if !self.closing {
            self.closing = true;
            self.shutdown.store(true, Ordering::Release);
            let _ = self.send(&json!({"type":"shutdown"}));
        }
    }
}

fn start_backend(shared: Shared, app: tauri::AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    // Tauri externalBin is placed alongside the app executable on each OS.
    // A fixed filename and embedded digest prevent PATH or user-selected code.
    let executable = std::env::current_exe()?;
    let base = executable.parent().ok_or("Application path unavailable")?;
    let backend = base.join(if cfg!(windows) { "xpotato-tunnel.exe" } else { "xpotato-tunnel" });
    if std::fs::symlink_metadata(&backend)?.file_type().is_symlink() { return Err("Backend link rejected".into()); }
    let mut file = File::open(&backend)?;
    let mut hash = Sha256::new();
    let mut buffer = [0u8; 65536];
    loop {
        let n = file.read(&mut buffer)?;
        if n == 0 { break; }
        hash.update(&buffer[..n]);
    }
    if format!("{:x}", hash.finalize()) != env!("XPOTATO_GUI_CORE_SHA") { return Err("Backend digest differs".into()); }
    let mut command = Command::new(backend);
    command.arg("_gui-bridge").stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::null());
    #[cfg(windows)] {
        use std::os::windows::process::CommandExt;
        command.creation_flags(0x08000000); // CREATE_NO_WINDOW, no console policy change
    }
    let mut child = command.spawn()?;
    let output = child.stdout.take().ok_or("Backend output unavailable")?;
    let mut input = child.stdin.take().ok_or("Backend input unavailable")?;
    let (sender, receiver) = sync_channel::<Vec<u8>>(8);
    let shutdown;
    {
        let mut state = shared.lock().map_err(|_| "Backend lock unavailable")?;
        shutdown = state.shutdown.clone();
        state.input = Some(sender);
        state.connected = true;
        state.alive = true;
    }
    // Never hold the state mutex during pipe I/O. Draining stdout remains
    // independent of a slow stdin, including poll/cancel/window-close paths.
    std::thread::spawn(move || {
        while let Ok(mut frame) = receiver.recv() {
            if shutdown.load(Ordering::Acquire) {
                frame.zeroize();
                let _ = input.write_all(b"{\"type\":\"shutdown\"}\n");
                break;
            }
            let result = input.write_all(&frame);
            frame.zeroize();
            if result.is_err() { break; }
        }
        while let Ok(mut frame) = receiver.try_recv() { frame.zeroize(); }
        // Dropping only this pipe triggers Go's EOF/drain path.
    });
    std::thread::spawn(move || {
        let mut reader = BufReader::new(output);
        loop {
            let mut frame = Vec::new();
            match reader.by_ref().take(LIMIT as u64).read_until(b'\n', &mut frame) {
                Ok(0) | Err(_) => break,
                Ok(_) => {}
            }
            if frame.len() >= LIMIT || frame.last() != Some(&b'\n') { break; }
            let Ok(event) = serde_json::from_slice::<Value>(&frame) else { break; };
            if let Ok(mut state) = shared.lock() { state.push(event); } else { break; }
        }
        if let Ok(mut state) = shared.lock() {
            state.connected = false;
            // Closing stdin asks only our bridge to cancel/drain its own work.
            state.input.take();
        }
        // Release stdout before wait: a malformed/oversized producer must get
        // a broken pipe, rather than block on an unread pipe while we wait.
        drop(reader);
        let status = child.wait();
        if let Ok(mut state) = shared.lock() {
            state.alive = false;
            if state.closing { app.exit(0); } else {
                state.push(json!({"type":"disconnected", "message": if status.map(|s| s.success()).unwrap_or(false) {"backend が終了しました。アプリを開き直してください。"} else {"backend が予期せず終了しました。アプリを開き直してください。"}}));
            }
        }
    });
    Ok(())
}

#[derive(Serialize)]
struct Poll { events: Vec<Value>, connected: bool, closing: bool }
#[tauri::command]
fn poll(state: State<'_, Shared>) -> Result<Poll, String> {
    let mut state = state.lock().map_err(|_| "内部状態を確認できません。")?;
    Ok(Poll { events: state.events.drain(..).collect(), connected: state.connected, closing: state.closing })
}
fn valid_id(id: &str) -> bool {
    !id.is_empty() && id.len() <= 64 && id.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'-')
}
#[tauri::command]
fn operate(id: String, action: String, home: String, device: String, state: State<'_, Shared>) -> Result<(), String> {
    if !valid_id(&id) || home.len()>4096 || device.len()>64 || !["setup","start","update","repair","doctor","deep","rollback","credentials","uninstall-entrypoints"].contains(&action.as_str()) {
        return Err("操作または入力が無効です。".into());
    }
    let mut state = state.lock().map_err(|_| "内部状態を確認できません。")?;
    if state.closing { return Err("終了処理中です。".into()); }
    state.send(&json!({"type":"operation","id":id,"action":action,"home":home,"device":device}))
}
#[tauri::command]
fn reply(id: String, prompt: u64, mut value: String, state: State<'_, Shared>) -> Result<(), String> {
    if !valid_id(&id) || value.len()>8192 { value.zeroize(); return Err("入力が無効です。".into()); }
    // Numeric bytes in the pipe, so Go can clear key buffers without creating
    // immutable secret strings. Never include a reply in an event/error/log.
    let mut bytes = value.as_bytes().to_vec();
    value.zeroize();
    let mut message = json!({"type":"reply","id":id,"prompt":prompt,"value":bytes});
    bytes.zeroize();
    let result = state.lock().map_err(|_| "内部状態を確認できません。")?.send(&message);
    if let Some(array) = message["value"].as_array_mut() { for item in array { *item = json!(0); } }
    result
}
#[tauri::command]
fn cancel(id: String, state: State<'_, Shared>) -> Result<(), String> {
    if !valid_id(&id) { return Err("操作が無効です。".into()); }
    state.lock().map_err(|_| "内部状態を確認できません。")?.send(&json!({"type":"cancel","id":id}))
}
#[tauri::command]
fn request_close(app: tauri::AppHandle, state: State<'_, Shared>) -> Result<(), String> {
    let mut state = state.lock().map_err(|_| "内部状態を確認できません。")?;
    if !state.alive { app.exit(0); } else { state.close(); }
    Ok(())
}
#[tauri::command]
async fn pick_folder(app: tauri::AppHandle) -> Result<Option<String>, String> {
    tauri::async_runtime::spawn_blocking(move || {
        app.dialog().file().set_title("許可するフォルダーを選択")
            .blocking_pick_folder().map(|path| path.into_path().map(|p| p.to_string_lossy().into_owned()).map_err(|_| "フォルダーを確認できません。".into())).transpose()
    }).await.map_err(|_| "フォルダー選択を完了できません。")?
}
fn main() {
    let shared = Shared::default();
    let setup_state = shared.clone();
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(shared)
        .setup(move |app| {
            if start_backend(setup_state.clone(), app.handle().clone()).is_err() {
                if let Ok(mut state) = setup_state.lock() {
                    state.push(json!({"type":"disconnected","message":"同梱 backend の確認・起動に失敗しました。正規の Installer でアプリを再配置してください。"}));
                }
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                let shared = window.state::<Shared>();
                if let Ok(mut state) = shared.lock() {
                    if state.alive { api.prevent_close(); state.close(); }
                };
            }
        })
        .invoke_handler(tauri::generate_handler![poll, operate, reply, cancel, request_close, pick_folder])
        .run(tauri::generate_context!())
        .expect("Desktop application could not start");
}
