use crate::state::{ServerState, Stats};
use crate::blocklist::load_blocklists_into;
use axum::{Json};
use serde_json::Value;
use std::sync::Arc;

pub async fn http_reload(state: Arc<ServerState>) -> Json<Value> {
    match load_blocklists_into("./blocklist", &state.lists).await {
        Ok(n) => {
            tracing::info!("reloaded {} domains", n);
            Json(serde_json::json!({ "loaded": n }))
        }
        Err(e) => {
            tracing::warn!("reload failed: {:?}", e);
            Json(serde_json::json!({ "error": format!("{}", e) }))
        }
    }
}

pub async fn http_stats(state: Arc<ServerState>) -> Json<Stats> {
    let q = state.queries.load(std::sync::atomic::Ordering::Relaxed);
    let b = state.blocked.load(std::sync::atomic::Ordering::Relaxed);
    Json(Stats { queries: q, blocked: b })
}

pub async fn http_lists(state: Arc<ServerState>) -> Json<Value> {
    let lists = state.lists.read().await;
    let v: Vec<String> = lists.iter().cloned().collect();
    Json(serde_json::json!({ "count": v.len(), "patterns": v }))
}

pub async fn http_add(state: Arc<ServerState>, Json(payload): Json<Value>) -> Json<Value> {
    if let Some(p) = payload.get("pattern").and_then(|s| s.as_str()) {
        let mut lists = state.lists.write().await;
        lists.insert(p.to_lowercase());
        Json(serde_json::json!({ "ok": true, "added": p }))
    } else {
        Json(serde_json::json!({ "ok": false, "error": "missing pattern" }))
    }
}

pub async fn http_remove(state: Arc<ServerState>, Json(payload): Json<Value>) -> Json<Value> {
    if let Some(p) = payload.get("pattern").and_then(|s| s.as_str()) {
        let mut lists = state.lists.write().await;
        let removed = lists.remove(&p.to_lowercase());
        Json(serde_json::json!({ "ok": removed }))
    } else {
        Json(serde_json::json!({ "ok": false, "error": "missing pattern" }))
    }
}

pub async fn http_mode(state: Arc<ServerState>, Json(payload): Json<Value>) -> Json<Value> {
    if let Some(m) = payload.get("mode").and_then(|s| s.as_str()) {
        let mut mode = state.mode.write().await;
        *mode = m.to_string();
        if m == "redirect" {
            if let Some(ip) = payload.get("block_ip").and_then(|s| s.as_str()) {
                let mut bip = state.block_page_ip.write().await;
                *bip = Some(ip.to_string());
            }
        }
        Json(serde_json::json!({ "ok": true, "mode": *mode }))
    } else {
        Json(serde_json::json!({ "ok": false, "error": "missing mode" }))
    }
}
