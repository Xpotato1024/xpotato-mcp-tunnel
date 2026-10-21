use sha2::{Digest, Sha256};
use std::{env, fs::File, io::Read, path::PathBuf};

fn main() {
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
