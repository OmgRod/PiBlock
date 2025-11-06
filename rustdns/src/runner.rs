use crate::state::ServerState;
use crate::blocklist::load_blocklists_into;
use crate::control::{http_reload, http_stats, http_lists, http_add, http_remove, http_mode};
use crate::server::run_udp_server;
use axum::{routing::get, routing::post, Router};
use std::collections::HashSet;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::sync::atomic::AtomicU64;
use tracing::info;

pub async fn run_server(http_addr: String, udp_bind: String, shutdown_rx: tokio::sync::watch::Receiver<bool>) {
    tracing_subscriber::fmt::init();

    let lists = Arc::new(RwLock::new(HashSet::new()));
    let state = Arc::new(ServerState {
        lists: lists.clone(),
        queries: Arc::new(AtomicU64::new(0)),
        blocked: Arc::new(AtomicU64::new(0)),
        upstream: "1.1.1.1:53".to_string(),
        mode: Arc::new(RwLock::new("nx".to_string())),
        block_page_ip: Arc::new(RwLock::new(None)),
    });

    // initial load
    if let Ok(n) = load_blocklists_into("./blocklist", &lists).await {
        info!("initially loaded {} domains", n);
    }

    // HTTP control plane
    let st_http = state.clone();
    let st_stats = state.clone();
    let st_lists = state.clone();
    let st_add = state.clone();
    let st_remove = state.clone();
    let st_mode = state.clone();
    let app = Router::new()
        .route("/reload", post(move || http_reload(st_http.clone())))
        .route("/stats", get(move || http_stats(st_stats.clone())))
        .route("/lists", get(move || http_lists(st_lists.clone())))
        .route("/add", post(move |b| http_add(st_add.clone(), b)))
        .route("/remove", post(move |b| http_remove(st_remove.clone(), b)))
        .route("/mode", post(move |b| http_mode(st_mode.clone(), b)));

    let http_addr: SocketAddr = http_addr.parse().unwrap_or_else(|_| "127.0.0.1:9080".parse().unwrap());
    let server = axum::Server::bind(&http_addr).serve(app.into_make_service());
    info!("control API listening on http://{}", http_addr);

    let udp_upstream = state.upstream.clone();

    // HTTP graceful shutdown
    let mut http_shutdown_rx = shutdown_rx.clone();
    let http_future = server.with_graceful_shutdown(async move {
        let _ = http_shutdown_rx.changed().await;
    });

    // UDP server runs in a task
    let udp_bind_owned = udp_bind.clone();
    let st_udp = state.clone();
    let udp_task = tokio::spawn(async move { run_udp_server(st_udp, udp_bind_owned, udp_upstream).await });

    let _ = tokio::join!(http_future, udp_task);
}
