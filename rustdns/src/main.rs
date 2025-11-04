mod blocklist;
mod control;
mod server;
mod state;
mod runner;

use anyhow::Result;
use std::env;

#[tokio::main]
async fn main() -> Result<()> {
    let http_addr = env::var("RUSTDNS_HTTP_ADDR").unwrap_or_else(|_| "127.0.0.1:9080".to_string());
    let udp_bind = env::var("RUSTDNS_UDP_BIND").unwrap_or_else(|_| "0.0.0.0:5353".to_string());

    // create shutdown channel (unused for simple binary run)
    let (_tx, rx) = tokio::sync::watch::channel(false);

    crate::runner::run_server(http_addr, udp_bind, rx).await;
    Ok(())
}
