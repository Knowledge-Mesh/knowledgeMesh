use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use tracing::info;

pub const DEFAULT_BROKER_URL: &str = "https://km-broker.onrender.com";

// ── Local config files (~/.km/configs/{node-name}.json) ─────────────

/// Returns the directory containing per-node config files.
pub fn configs_dir() -> std::path::PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    std::path::PathBuf::from(home).join(".km").join("configs")
}

/// Returns the config file path for a specific node.
pub fn config_path_for(node_name: &str) -> std::path::PathBuf {
    configs_dir().join(format!("{}.json", node_name))
}

/// Legacy config path for migration.
fn legacy_config_path() -> std::path::PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    std::path::PathBuf::from(home).join(".km").join("config.json")
}

#[derive(Serialize, Deserialize, Debug)]
pub struct LocalConfig {
    pub node_name: String,
    pub node_secret: String,
    pub tier: String,
    pub price: f64,
    pub broker_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub email: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token_budget: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub budget_window_hours: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_concurrent: Option<i32>,
}

/// Load a local config. Resolution order:
/// 1. If KM_NODE_NAME is set, load `~/.km/configs/{node_name}.json`.
/// 2. If exactly one config file exists in `~/.km/configs/`, use it.
/// 3. If multiple exist, log them and return None (user must set KM_NODE_NAME).
/// 4. Fall back to legacy `~/.km/config.json` for migration.
pub fn load_local_config() -> Option<LocalConfig> {
    // If KM_NODE_NAME is set, load that specific config
    if let Ok(node_name) = std::env::var("KM_NODE_NAME") {
        let path = config_path_for(&node_name);
        if let Ok(data) = std::fs::read_to_string(&path) {
            return serde_json::from_str(&data).ok();
        }
    }

    // Scan ~/.km/configs/ directory
    let dir = configs_dir();
    if let Ok(entries) = std::fs::read_dir(&dir) {
        let configs: Vec<std::path::PathBuf> = entries
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .filter(|p| p.extension().and_then(|e| e.to_str()) == Some("json"))
            .collect();

        match configs.len() {
            0 => {} // fall through to legacy
            1 => {
                if let Ok(data) = std::fs::read_to_string(&configs[0]) {
                    return serde_json::from_str(&data).ok();
                }
            }
            _ => {
                eprintln!("Multiple node configs found in {}:", dir.display());
                for p in &configs {
                    if let Some(stem) = p.file_stem().and_then(|s| s.to_str()) {
                        eprintln!("  - {}", stem);
                    }
                }
                eprintln!("Set KM_NODE_NAME to specify which node to run.");
                return None;
            }
        }
    }

    // Legacy fallback: ~/.km/config.json
    let legacy = legacy_config_path();
    if let Ok(data) = std::fs::read_to_string(&legacy) {
        if let Ok(config) = serde_json::from_str::<LocalConfig>(&data) {
            info!("[config] Migrating legacy config to ~/.km/configs/{}.json", config.node_name);
            // Migrate: save to new location, remove legacy
            if save_local_config(&config).is_ok() {
                let _ = std::fs::remove_file(&legacy);
            }
            return Some(config);
        }
    }

    None
}

pub fn save_local_config(config: &LocalConfig) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;

    let dir = configs_dir();
    std::fs::create_dir_all(&dir)
        .context("Failed to create ~/.km/configs directory")?;
    // Restrict directory to owner only (rwx------)
    std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o700))
        .context("Failed to set ~/.km/configs directory permissions to 0700")?;
    // Also ensure parent ~/.km has correct permissions
    if let Some(parent) = dir.parent() {
        std::fs::set_permissions(parent, std::fs::Permissions::from_mode(0o700))
            .context("Failed to set ~/.km directory permissions to 0700")?;
    }

    let path = config_path_for(&config.node_name);
    let json = serde_json::to_string_pretty(config)?;
    std::fs::write(&path, &json)
        .with_context(|| format!("Failed to write {}", path.display()))?;
    // Restrict config file to owner read/write only (rw-------)
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o600))
        .with_context(|| format!("Failed to set {} permissions to 0600", path.display()))?;
    info!("[config] Saved config to {} (mode 0600)", path.display());
    Ok(())
}

// ── Config fetch from broker ────────────────────────────────────────

#[derive(Deserialize)]
pub struct NodeConfigResponse {
    pub name: String,
    pub tier: String,
    pub price_per_million_tokens: f64,
    pub broker_url: String,
    #[allow(dead_code)]
    pub node_secret: String,
}

pub async fn fetch_config_from_broker(node_secret: &str) -> Result<NodeConfigResponse> {
    let broker_url = std::env::var("KM_BROKER_URL")
        .unwrap_or_else(|_| DEFAULT_BROKER_URL.to_string());

    let url = format!("{}/node-config", broker_url);
    info!("[config] Fetching node config from broker...");

    let client = reqwest::Client::new();
    let resp = client.get(&url)
        .header("Authorization", format!("Bearer {}", node_secret))
        .send().await
        .context("Failed to reach broker for config. Is the broker running?")?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("Broker returned error: {}", text);
    }

    let config: NodeConfigResponse = resp.json().await
        .context("Failed to parse broker config response")?;

    info!("[config] Got config: name={}, tier={}, price={}/M", config.name, config.tier, config.price_per_million_tokens);
    Ok(config)
}

// ── Bootstrap — sets env vars from broker config ────────────────────

pub async fn bootstrap_from_broker() -> Result<bool> {
    let node_secret = match std::env::var("KM_NODE_SECRET") {
        Ok(s) if !s.is_empty() => s,
        _ => return Ok(false), // No secret — manual mode
    };

    // If KM_TIER is already explicitly set, user is in manual mode
    if std::env::var("KM_TIER").is_ok() {
        return Ok(false);
    }

    let config = fetch_config_from_broker(&node_secret).await?;

    // Set env vars from broker config (only if not already set)
    if std::env::var("KM_BROKER_URL").is_err() {
        std::env::set_var("KM_BROKER_URL", &config.broker_url);
    }
    if std::env::var("KM_NODE_NAME").is_err() {
        std::env::set_var("KM_NODE_NAME", &config.name);
    }
    if std::env::var("KM_PRICE").is_err() {
        std::env::set_var("KM_PRICE", config.price_per_million_tokens.to_string());
    }
    std::env::set_var("KM_TIER", &config.tier);

    Ok(true)
}
