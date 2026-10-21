use sha2::{Digest, Sha256};
use std::{env, fs::File, io::Read, path::PathBuf};

fn main() {
    println!("cargo:rerun-if-env-changed=XPOTATO_BUILD_COMMIT");
    println!("cargo:rerun-if-env-changed=XPOTATO_BUILD_CLEAN");
    let commit = env::var("XPOTATO_BUILD_COMMIT").unwrap_or_else(|_| "uncommitted".into());
    println!("cargo:rustc-env=XPOTATO_GUI_COMMIT={commit}");
    println!("cargo:rustc-env=XPOTATO_GUI_CLEAN={}",env::var("XPOTATO_BUILD_CLEAN").unwrap_or_else(|_| "0".into()));
    let target = env::var("TARGET").expect("Cargo target missing");
    let extension = if target.contains("windows") { ".exe" } else { "" };
    let backend = PathBuf::from("binaries").join(format!("xpotato-tunnel-{target}{extension}"));
    println!("cargo:rerun-if-changed={}", backend.display());
    let mut file = File::open(&backend).expect("build the matching Go sidecar first");
    let mut hash = Sha256::new();
    let mut buffer = [0u8; 65536];
    loop {
        let n = file.read(&mut buffer).expect("read Go sidecar");
        if n == 0 { break; }
        hash.update(&buffer[..n]);
    }
    println!("cargo:rustc-env=XPOTATO_GUI_CORE_SHA={:x}", hash.finalize());
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&["poll", "operate", "reply", "cancel", "request_close", "pick_folder"])
    )).expect("Tauri configuration invalid");
}
