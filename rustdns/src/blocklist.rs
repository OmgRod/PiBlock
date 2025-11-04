use anyhow::Result;
use glob::glob;
use std::collections::HashSet;
use std::sync::Arc;
use tokio::sync::RwLock;

// Load all .txt files from `dir` into the provided `lists` set. Accepts hosts-style and plain lines.
pub async fn load_blocklists_into(dir: &str, lists: &Arc<RwLock<HashSet<String>>>) -> Result<usize> {
    let mut set = HashSet::new();
    let pattern = format!("{}/*.txt", dir);
    for entry in glob(&pattern)? {
        if let Ok(path) = entry {
            if path.is_file() {
                if let Ok(s) = tokio::fs::read_to_string(&path).await {
                    for line in s.lines() {
                        let line = line.trim();
                        if line.is_empty() || line.starts_with('#') { continue }
                        // accept hosts-style (ip domain) or plain domain
                        let domain = if line.contains(char::is_whitespace) {
                            line.split_whitespace().last().unwrap_or(line)
                        } else { line };
                        let d = domain.trim().to_lowercase();
                        if !d.is_empty() { set.insert(d); }
                    }
                }
            }
        }
    }
    let n = set.len();
    let mut w = lists.write().await;
    *w = set;
    Ok(n)
}

// Very simple matching: exact match or prefix/suffix wildcard patterns used in the lists.
pub fn is_blocked_domain(name: &str, lists: &HashSet<String>) -> bool {
    let name = name.trim_end_matches('.').to_lowercase();
    if lists.contains(&name) { return true }
    for pat in lists.iter() {
        if pat.starts_with("*.") {
            let suffix = &pat[2..];
            if name.ends_with(suffix) { return true }
        } else if pat.ends_with(".*") {
            let prefix = &pat[..pat.len()-2];
            if name.starts_with(prefix) { return true }
        }
    }
    false
}
