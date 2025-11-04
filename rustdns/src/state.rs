use serde::Serialize;
use std::collections::HashSet;
use std::sync::Arc;
use std::sync::atomic::AtomicU64;
use tokio::sync::RwLock;

#[derive(Clone)]
pub struct ServerState {
    pub lists: Arc<RwLock<HashSet<String>>>,
    pub queries: Arc<AtomicU64>,
    pub blocked: Arc<AtomicU64>,
    pub upstream: String,
    pub mode: Arc<RwLock<String>>,
    pub block_page_ip: Arc<RwLock<Option<String>>>,
}

#[derive(Serialize)]
pub struct Stats {
    pub queries: u64,
    pub blocked: u64,
}
