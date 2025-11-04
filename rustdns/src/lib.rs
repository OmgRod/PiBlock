mod blocklist;
mod control;
mod server;
mod state;
mod runner;

use std::ffi::CStr;
use std::os::raw::c_char;
use std::sync::{Mutex, Once};
use once_cell::sync::OnceCell;
use std::thread::{self, JoinHandle};

static START_ONCE: Once = Once::new();
static THREAD_HANDLE: OnceCell<Mutex<Option<JoinHandle<()>>>> = OnceCell::new();
static SHUTDOWN_TX: OnceCell<tokio::sync::watch::Sender<bool>> = OnceCell::new();

fn init_cells() {
    START_ONCE.call_once(|| {
        THREAD_HANDLE.set(Mutex::new(None)).ok();
    });
}

#[no_mangle]
pub extern "C" fn rustdns_start(http_addr: *const c_char, udp_bind: *const c_char) -> i32 {
    init_cells();
    let http = if http_addr.is_null() { "127.0.0.1:8082".to_string() } else {
        unsafe { CStr::from_ptr(http_addr).to_string_lossy().into_owned() }
    };
    let udp = if udp_bind.is_null() { "0.0.0.0:5353".to_string() } else {
        unsafe { CStr::from_ptr(udp_bind).to_string_lossy().into_owned() }
    };

    // if already started, return 1
    if THREAD_HANDLE.get().and_then(|m| m.lock().ok()).and_then(|g| g.as_ref().map(|_| ())).is_some() {
        return 1;
    }

    // create shutdown channel
    let (tx, mut rx) = tokio::sync::watch::channel(false);
    let _ = SHUTDOWN_TX.set(tx.clone());

    // spawn thread that runs tokio runtime
    let handle = thread::spawn(move || {
        let rt = tokio::runtime::Runtime::new().expect("tokio runtime");
        rt.block_on(async move {
                // call into runner::run_server
                crate::runner::run_server(http, udp, rx.clone()).await;
            });
    });

    if let Some(m) = THREAD_HANDLE.get() {
        if let Ok(mut guard) = m.lock() {
            *guard = Some(handle);
        }
    }
    0
}

#[no_mangle]
pub extern "C" fn rustdns_stop() -> i32 {
    if let Some(tx) = SHUTDOWN_TX.get() {
        if tx.send(true).is_ok() {
            // try to join thread
            if let Some(m) = THREAD_HANDLE.get() {
                if let Ok(mut guard) = m.lock() {
                    if let Some(h) = guard.take() {
                        let _ = h.join();
                    }
                }
            }
            return 0;
        }
    }
    1
}
