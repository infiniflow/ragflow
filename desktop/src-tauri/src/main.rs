//! RAGFlow desktop shell.
//!
//! The shell is a window and a process supervisor, nothing else. All product
//! logic lives in the Go sidecar (the `desktop` build of `ragflow_server.go`),
//! which serves the bundled web frontend on a loopback port and publishes that
//! URL in a ready file. The shell waits for the file, points the window at it,
//! and kills the sidecar when the window closes.
//!
//! Keeping the shell this thin is what keeps the app small: no bundled Chromium
//! (the system WebView does the rendering) and no duplicated assets (the Go
//! binary carries the frontend).

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};

/// How long the shell waits for the engine to publish its UI URL. First run has
/// to migrate the store and start the in-process backends, so this is generous.
const READY_TIMEOUT: Duration = Duration::from_secs(60);

/// CREATE_NO_WINDOW: start a console-subsystem child without giving it a console
/// window. Windows has no userland equivalent of "run this quietly" for a
/// console binary, so the flag is spelled out here rather than pulled in from a
/// crate for one constant.
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// Owns the sidecar process so the exit handler can stop it.
struct Sidecar(Arc<Mutex<Option<Child>>>);

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let data_dir = data_dir(app.handle())?;
            std::fs::create_dir_all(&data_dir)?;
            let ready_file = ready_file(app.handle())?;
            // A leftover file from a previous run would be read as this run's URL.
            let _ = std::fs::remove_file(&ready_file);

            let child = Arc::new(Mutex::new(Some(spawn_sidecar(app.handle(), &data_dir, &ready_file)?)));
            app.manage(Sidecar(child.clone()));

            // The window exists before the engine is ready, showing the bundled
            // placeholder; it is then redirected to the app's own origin.
            let window = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
                .title("RAGFlow")
                .inner_size(1280.0, 840.0)
                .min_inner_size(960.0, 640.0)
                .build()?;

            let handle = app.handle().clone();
            std::thread::spawn(move || {
                match wait_for_url(&ready_file, READY_TIMEOUT) {
                    Some(url) => {
                        let script = format!("window.location.replace({url:?});");
                        if let Err(err) = window.eval(&script) {
                            eprintln!("ragflow: could not open {url}: {err}");
                        }
                    }
                    None => eprintln!(
                        "ragflow: the engine did not publish a UI URL within {}s; see the log next to the data directory",
                        READY_TIMEOUT.as_secs()
                    ),
                }
                // A dead sidecar means a dead app: leaving the window up would
                // show connection errors forever.
                loop {
                    std::thread::sleep(Duration::from_secs(2));
                    let running = child
                        .lock()
                        .ok()
                        .and_then(|mut guard| guard.as_mut().map(|c| matches!(c.try_wait(), Ok(None))))
                        .unwrap_or(false);
                    if !running {
                        handle.exit(1);
                        return;
                    }
                }
            });
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("build the RAGFlow window")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                if let Some(state) = app.try_state::<Sidecar>() {
                    if let Ok(mut guard) = state.0.lock() {
                        if let Some(mut child) = guard.take() {
                            let _ = child.kill();
                            let _ = child.wait();
                        }
                    }
                }
            }
        });
}

/// data_dir is the engine's root: `RAGFLOW_DATA_DIR` overrides the platform
/// application-data directory, which is what a packaged app should use.
fn data_dir(app: &tauri::AppHandle) -> Result<PathBuf, Box<dyn std::error::Error>> {
    if let Ok(dir) = std::env::var("RAGFLOW_DATA_DIR") {
        if !dir.is_empty() {
            return Ok(PathBuf::from(dir));
        }
    }
    Ok(app.path().app_local_data_dir()?.join("data"))
}

fn ready_file(app: &tauri::AppHandle) -> Result<PathBuf, Box<dyn std::error::Error>> {
    let dir = app.path().app_cache_dir()?;
    std::fs::create_dir_all(&dir)?;
    Ok(dir.join("ui-ready.json"))
}

/// The sidecar is bundled as a resource next to the shell (`binaries/` inside
/// the macOS bundle's Resources, the resource dir on Windows and Linux).
/// `RAGFLOW_SIDECAR` points the shell at a locally built binary during
/// development.
fn spawn_sidecar(
    app: &tauri::AppHandle,
    data_dir: &Path,
    ready_file: &Path,
) -> Result<Child, Box<dyn std::error::Error>> {
    let exe = sidecar_path(app)?;
    let log = app.path().app_log_dir()?;
    std::fs::create_dir_all(&log)?;
    let log = std::fs::OpenOptions::new().create(true).append(true).open(log.join("engine.log"))?;

    let mut command = Command::new(&exe);
    command
        .arg("--ui")
        .arg("--data-dir")
        .arg(data_dir)
        .arg("--ui-ready-file")
        .arg(ready_file)
        .stdin(Stdio::null())
        .stdout(Stdio::from(log.try_clone()?))
        .stderr(Stdio::from(log));

    // A window launched from Finder starts in "/": nothing can be written there,
    // and the engine's cwd-relative defaults (its `logs` directory among them)
    // have no home. Run it from the data directory instead.
    command.current_dir(data_dir);

    // The engine is a console-subsystem binary. Launched from this GUI-subsystem
    // shell it would otherwise get a console window of its own — and closing that
    // console kills the engine, which the shell treats as "the app is gone".
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        command.creation_flags(CREATE_NO_WINDOW);
    }

    // The analyzer dictionaries and the configuration directory ship as
    // resources. Say where they are rather than let the engine guess: on Windows
    // and Linux the binary and the resources live in different directories, so
    // its executable-relative lookup cannot find them.
    let resources = app.path().resource_dir()?;
    for (key, dir) in [
        ("RAGFLOW_DICT_PATH", resources.join("resource")),
        ("RAGFLOW_CONF_DIR", resources.join("conf")),
        ("DEEPDOC_MODEL_DIR", resources.join("deepdoc")),
    ] {
        if dir.is_dir() {
            command.env(key, dir);
        }
    }

    let child = command
        .spawn()
        .map_err(|err| format!("start {}: {err}", exe.display()))?;
    Ok(child)
}

fn sidecar_path(app: &tauri::AppHandle) -> Result<PathBuf, Box<dyn std::error::Error>> {
    if let Ok(path) = std::env::var("RAGFLOW_SIDECAR") {
        if !path.is_empty() {
            return Ok(PathBuf::from(path));
        }
    }
    let resources = app.path().resource_dir()?;
    let names: [&str; 2] = if cfg!(windows) {
        ["binaries/ragflow-desktop.exe", "binaries/ragflow-desktop"]
    } else {
        ["binaries/ragflow-desktop", "binaries/ragflow-desktop.exe"]
    };
    for name in names {
        let candidate = resources.join(name);
        if candidate.is_file() {
            return Ok(candidate);
        }
    }
    Err(format!(
        "the engine binary is missing from {}; rebuild the app, or set RAGFLOW_SIDECAR",
        resources.display()
    )
    .into())
}

/// Poll the ready file. The writer publishes it with a temp-and-rename, but a
/// torn read would only cost one poll, so a parse failure is simply retried.
fn wait_for_url(ready_file: &Path, timeout: Duration) -> Option<String> {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if let Ok(raw) = std::fs::read(ready_file) {
            if let Ok(value) = serde_json::from_slice::<serde_json::Value>(&raw) {
                if let Some(url) = value.get("url").and_then(|v| v.as_str()) {
                    if url.starts_with("http://") {
                        return Some(url.to_string());
                    }
                }
            }
        }
        std::thread::sleep(Duration::from_millis(250));
    }
    None
}
