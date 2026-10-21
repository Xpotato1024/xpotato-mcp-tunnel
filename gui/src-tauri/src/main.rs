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
            if let Some(index) = self.events.iter().position(|item| item["type"] == "progress" || item["type"] == "working") {
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

fn validate_backend(backend: &std::path::Path, expected: &str) -> Result<(), Box<dyn std::error::Error>> {
    let metadata=std::fs::symlink_metadata(backend)?;
    if !metadata.is_file() || metadata.file_type().is_symlink() { return Err("Backend file rejected".into()); }
    let mut file = File::open(backend)?;
    let mut hash = Sha256::new();
    let mut buffer = [0u8; 65536];
    loop {
        let n = file.read(&mut buffer)?;
        if n == 0 { break; }
        hash.update(&buffer[..n]);
    }
    if format!("{:x}", hash.finalize()) != expected { return Err("Backend digest differs".into()); }
    Ok(())
}
fn start_backend(shared: Shared, app: tauri::AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    // Fixed adjacent filename plus embedded digest: never PATH/user-selected code.
    let executable = std::env::current_exe()?;
    let base = executable.parent().ok_or("Application path unavailable")?;
    let backend = base.join(if cfg!(windows) { "xpotato-tunnel.exe" } else { "xpotato-tunnel" });
    validate_backend(&backend,env!("XPOTATO_GUI_CORE_SHA"))?;
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
    let result = match state.lock() {
        Ok(mut state) => state.send(&message),
        Err(_) => Err("内部状態を確認できません。".into()),
    };
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
    // Build-time packaging inquiry only; never opens a WebView, Vault or core.
    if std::env::args().skip(1).collect::<Vec<_>>() == ["--build-info"] {
        #[allow(unused_mut)]
        let mut info=json!({"version":env!("CARGO_PKG_VERSION"),"commit":env!("XPOTATO_GUI_COMMIT"),"dirty":env!("XPOTATO_GUI_CLEAN")!="1","coreSHA256":env!("XPOTATO_GUI_CORE_SHA"),"nativeFixture":cfg!(feature="native-fixture")});
        #[cfg(feature="native-fixture")]
        { info["webviewVersion"]=json!(tauri::webview_version().ok()); }
        println!("{info}");
        return;
    }
    #[allow(unused_mut)]
    let mut context = tauri::generate_context!();
    // Elevated WebView2 hosts ignore environment overrides. Use API options
    // only in a separately compiled, non-distributable CI harness. Production
    // has no code for this branch and never reads these fixture variables.
    #[cfg(all(windows,feature="native-fixture"))]
    {
        let port: u16 = std::env::var("XPOTATO_FIXTURE_PORT").expect("fixture port").parse().expect("numeric fixture port");
        assert!(port > 0);
        let profile = std::path::PathBuf::from(std::env::var_os("XPOTATO_FIXTURE_PROFILE").expect("fixture profile"));
        assert!(profile.is_absolute());
        let window = &mut context.config_mut().app.windows[0];
        window.additional_browser_args = Some(format!("--remote-debugging-port={port} --remote-debugging-address=127.0.0.1"));
        window.data_directory = Some(profile);
    }
    let shared = Shared::default();
    let setup_state = shared.clone();
    let result=tauri::Builder::default()
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
        .run(context);
    if let Err(_error) = result {
        eprintln!("Xpotato Tunnel could not start; check the OS WebView and application installation.");
        #[cfg(feature="native-fixture")]
        eprintln!("native fixture initialization: {_error:?}");
        // This path precedes any WebView UI; give a visible Windows error even
        // when the system WebView2 runtime is missing. No download/elevation.
        #[cfg(windows)] unsafe {
            #[link(name="user32")]
            extern "system" {fn MessageBoxW(window:*mut std::ffi::c_void,text:*const u16,title:*const u16,flags:u32)->i32;}
            let message:Vec<u16>="アプリを起動できません。Windows の WebView2 Evergreen が利用可能か、正規 Installer で配置した版か確認してください。OS の設定変更や自動ダウンロードは行っていません。".encode_utf16().chain(Some(0)).collect();
            let title:Vec<u16>="Xpotato Tunnel".encode_utf16().chain(Some(0)).collect();
            MessageBoxW(std::ptr::null_mut(),message.as_ptr(),title.as_ptr(),0x10);
        }
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn sidecar_digest_rejects_tampering_before_any_execution() {
        let path=std::env::temp_dir().join(format!("xpotato-gui-digest-{}",std::process::id()));
        let mut file=std::fs::OpenOptions::new().write(true).create_new(true).open(&path).unwrap();
        file.write_all(b"fixture backend").unwrap();drop(file);
        let digest=format!("{:x}",Sha256::digest(b"fixture backend"));
        assert!(validate_backend(&path,&digest).is_ok());
        assert!(validate_backend(&path,&"0".repeat(64)).is_err());
        std::fs::write(&path,b"modified").unwrap();
        assert!(validate_backend(&path,&digest).is_err());
        std::fs::remove_file(path).unwrap();
    }
    #[test]
    fn bounded_queue_does_not_block_cancel_or_close() {
        let (tx,rx)=sync_channel(1);
        let mut state=Backend::default();state.input=Some(tx);
        state.send(&json!({"type":"operation","id":"own-1"})).unwrap();
        assert!(state.send(&json!({"type":"cancel","id":"own-1"})).is_err());
        state.close(); // Queue is full, but priority shutdown is still visible.
        assert!(state.shutdown.load(Ordering::Acquire));
        assert!(state.closing);
        assert_eq!(serde_json::from_slice::<Value>(&rx.recv().unwrap()).unwrap()["id"],"own-1");
    }
    #[test]
    fn oversized_frames_and_unrelated_ids_are_rejected() {
        let (tx,rx)=sync_channel(1);
        let mut state=Backend::default();state.input=Some(tx);
        assert!(state.send(&json!({"value":"x".repeat(LIMIT)})).is_err());
        assert!(rx.try_recv().is_err());
        assert!(!valid_id("../other"));assert!(!valid_id(""));assert!(valid_id("own-123"));
    }
    #[test]
    fn slow_poll_preserves_prompt_and_terminal_events() {
        let mut state=Backend::default();
        for _ in 0..256 {state.push(json!({"type":"working"}));}
        state.push(json!({"type":"prompt","id":"own"}));
        state.push(json!({"type":"complete","id":"own"}));
        assert_eq!(state.events.len(),256);
        assert!(state.events.iter().any(|e|e["type"]=="prompt"));
        assert!(state.events.iter().any(|e|e["type"]=="complete"));
    }
}
