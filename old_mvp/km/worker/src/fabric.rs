use std::sync::Arc;

use anyhow::{Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use tokio::sync::{watch, Mutex, RwLock};
use tracing::{error, info, warn};

use crate::bridge::SupplyTier;
use crate::config;

const HEARTBEAT_INTERVAL_SECS: u64 = 10;

#[derive(Serialize)]
struct RegisterBody {
    name: String,
    tier: String,
    price_per_million_tokens: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    models: Option<std::collections::HashMap<String, f64>>,
    tunnel_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    node_secret: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    token_budget: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    budget_window_hours: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_concurrent: Option<i32>,
}

#[derive(Deserialize)]
struct RegisterResponse {
    node_id: String,
}

#[derive(Serialize)]
struct HeartbeatBody {
    node_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    node_secret: Option<String>,
}

#[derive(Serialize)]
struct RecoverBody {
    name: String,
    email: String,
}

#[derive(Deserialize)]
struct RecoverResponse {
    reset_token: Option<String>,
    message: Option<String>,
}

#[derive(Serialize)]
struct ResetSecretBody {
    reset_token: String,
}

#[derive(Deserialize)]
struct ResetSecretResponse {
    node_secret: String,
}

pub struct FabricClient {
    client: Client,
    broker_url: String,
    node_name: String,
    tier: String,
    price: f64,
    models: Option<std::collections::HashMap<String, f64>>,
    node_secret: Arc<RwLock<Option<String>>>,
    token_budget: Option<i64>,
    budget_window_hours: Option<i32>,
    max_concurrent: Option<i32>,
    tunnel_url: Arc<RwLock<String>>,
    node_id: Arc<Mutex<Option<String>>>,
}

impl FabricClient {
    pub fn new(
        broker_url: String,
        node_name: String,
        tier: SupplyTier,
        price: f64,
        models: Option<std::collections::HashMap<String, f64>>,
        tunnel_url: String,
        node_secret: Option<String>,
        token_budget: Option<i64>,
        budget_window_hours: Option<i32>,
        max_concurrent: Option<i32>,
    ) -> Self {
        Self {
            client: Client::new(),
            broker_url,
            node_name,
            tier: tier.to_string(),
            price,
            models,
            node_secret: Arc::new(RwLock::new(node_secret)),
            token_budget,
            budget_window_hours,
            max_concurrent,
            tunnel_url: Arc::new(RwLock::new(tunnel_url)),
            node_id: Arc::new(Mutex::new(None)),
        }
    }

    async fn register(&self) -> Result<String> {
        let url = format!("{}/register", self.broker_url);
        let current_tunnel_url = self.tunnel_url.read().await.clone();
        let current_secret = self.node_secret.read().await.clone();
        let body = RegisterBody {
            name: self.node_name.clone(),
            tier: self.tier.clone(),
            price_per_million_tokens: self.price,
            models: self.models.clone(),
            tunnel_url: current_tunnel_url,
            node_secret: current_secret,
            token_budget: self.token_budget,
            budget_window_hours: self.budget_window_hours,
            max_concurrent: self.max_concurrent,
        };

        let resp = self
            .client
            .post(&url)
            .json(&body)
            .send()
            .await
            .context("Failed to reach broker for registration")?;

        let status = resp.status();
        let text = resp.text().await?;

        if status.as_u16() == 403 && text.to_lowercase().contains("not registered") {
            warn!("[fabric] Node not registered on broker — attempting recovery...");
            return self.attempt_recovery_and_reregister().await;
        }

        if !status.is_success() {
            anyhow::bail!("Broker registration failed ({}): {}", status, text);
        }

        let parsed: RegisterResponse = serde_json::from_str(&text)?;
        info!("[fabric] Registered with broker as {} ({})", self.node_name, parsed.node_id);
        Ok(parsed.node_id)
    }

    async fn heartbeat(&self, node_id: &str) -> Result<()> {
        let url = format!("{}/heartbeat", self.broker_url);
        let current_secret = self.node_secret.read().await.clone();
        let body = HeartbeatBody {
            node_id: node_id.to_string(),
            node_secret: current_secret,
        };

        let resp = self.client.post(&url).json(&body).send().await?;

        if resp.status().as_u16() == 404 {
            anyhow::bail!("Node unknown — need to re-register");
        }

        if !resp.status().is_success() {
            let text = resp.text().await.unwrap_or_default();
            anyhow::bail!("Heartbeat failed: {}", text);
        }

        Ok(())
    }

    /// Attempt to recover a lost registration by requesting a reset token from the broker,
    /// then resetting the node secret, updating local config and env var, and re-registering.
    async fn attempt_recovery_and_reregister(&self) -> Result<String> {
        // Load email from saved config
        let local_cfg = config::load_local_config();
        let email = local_cfg
            .as_ref()
            .and_then(|c| c.email.clone())
            .filter(|e| !e.is_empty());

        let email = match email {
            Some(e) => e,
            None => {
                anyhow::bail!(
                    "Registration lost and no email on file for recovery.\n\
                     Re-register with: KM_INVITE_CODE=xxx KM_NODE_NAME=xxx km-worker"
                );
            }
        };

        // Step 1: POST /recover to get a reset token
        info!("[fabric] Requesting recovery token for {} ({})", self.node_name, email);
        let recover_resp = self
            .client
            .post(&format!("{}/recover", self.broker_url))
            .json(&RecoverBody {
                name: self.node_name.clone(),
                email: email.clone(),
            })
            .send()
            .await
            .context("Failed to reach broker for recovery")?;

        if !recover_resp.status().is_success() {
            let text = recover_resp.text().await.unwrap_or_default();
            anyhow::bail!(
                "Recovery failed ({}). Re-register with: KM_INVITE_CODE=xxx KM_NODE_NAME=xxx km-worker",
                text
            );
        }

        let recover_data: RecoverResponse = recover_resp
            .json()
            .await
            .context("Failed to parse recovery response")?;

        let reset_token = match recover_data.reset_token {
            Some(t) => t,
            None => {
                // Email was sent instead of returning token directly
                let msg = recover_data.message.unwrap_or_default();
                anyhow::bail!(
                    "Recovery requires email verification: {}. \
                     Then run: POST /reset-secret with the token from your email.",
                    msg
                );
            }
        };

        // Step 2: POST /reset-secret with the token
        info!("[fabric] Resetting node secret via recovery token...");
        let reset_resp = self
            .client
            .post(&format!("{}/reset-secret", self.broker_url))
            .json(&ResetSecretBody {
                reset_token,
            })
            .send()
            .await
            .context("Failed to reach broker for secret reset")?;

        if !reset_resp.status().is_success() {
            let text = reset_resp.text().await.unwrap_or_default();
            anyhow::bail!(
                "Secret reset failed ({}). Re-register with: KM_INVITE_CODE=xxx KM_NODE_NAME=xxx km-worker",
                text
            );
        }

        let reset_data: ResetSecretResponse = reset_resp
            .json()
            .await
            .context("Failed to parse reset-secret response")?;

        let new_secret = reset_data.node_secret;

        // Step 3: Update in-memory secret
        {
            let mut secret_lock = self.node_secret.write().await;
            *secret_lock = Some(new_secret.clone());
        }

        // Step 4: Update env var so the rest of the process sees it
        std::env::set_var("KM_NODE_SECRET", &new_secret);

        // Step 5: Update local config file
        if let Some(mut cfg) = local_cfg {
            cfg.node_secret = new_secret.clone();
            if let Err(e) = config::save_local_config(&cfg) {
                warn!("[fabric] Failed to save updated config: {}", e);
            } else {
                info!("[fabric] Local config updated with new secret");
            }
        }

        // Step 6: Retry registration with the new secret
        info!("[fabric] Retrying registration with recovered credentials...");
        let current_tunnel_url = self.tunnel_url.read().await.clone();
        let body = RegisterBody {
            name: self.node_name.clone(),
            tier: self.tier.clone(),
            price_per_million_tokens: self.price,
            models: self.models.clone(),
            tunnel_url: current_tunnel_url,
            node_secret: Some(new_secret),
            token_budget: self.token_budget,
            budget_window_hours: self.budget_window_hours,
            max_concurrent: self.max_concurrent,
        };

        let resp = self
            .client
            .post(&format!("{}/register", self.broker_url))
            .json(&body)
            .send()
            .await
            .context("Recovery re-registration failed to reach broker")?;

        let status = resp.status();
        let text = resp.text().await?;

        if !status.is_success() {
            anyhow::bail!("Recovery re-registration failed ({}): {}", status, text);
        }

        let parsed: RegisterResponse = serde_json::from_str(&text)?;
        info!("[fabric] Recovery successful — re-registered as {} ({})", self.node_name, parsed.node_id);
        Ok(parsed.node_id)
    }

    pub async fn start(self: Arc<Self>, tunnel_url_rx: Option<watch::Receiver<String>>) -> Result<()> {
        // Initial registration
        let id = self.register().await?;
        *self.node_id.lock().await = Some(id);

        // Spawn heartbeat loop
        let client = self.clone();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(
                std::time::Duration::from_secs(HEARTBEAT_INTERVAL_SECS),
            );

            loop {
                interval.tick().await;

                let node_id = {
                    let lock = client.node_id.lock().await;
                    lock.clone()
                };

                if let Some(ref id) = node_id {
                    if let Err(e) = client.heartbeat(id).await {
                        warn!("[fabric] Heartbeat failed: {} — re-registering", e);
                        match client.register().await {
                            Ok(new_id) => {
                                *client.node_id.lock().await = Some(new_id);
                            }
                            Err(e) => {
                                error!("[fabric] Re-registration failed: {} — will retry next heartbeat", e);
                            }
                        }
                    }
                }
            }
        });

        // Spawn tunnel URL watcher — re-registers when tunnel reconnects with new URL
        if let Some(mut rx) = tunnel_url_rx {
            let client = self.clone();
            tokio::spawn(async move {
                loop {
                    if rx.changed().await.is_err() {
                        break; // Tunnel handle dropped
                    }
                    let new_url = rx.borrow().clone();
                    info!("[fabric] Tunnel URL changed to {} — re-registering with broker", new_url);
                    *client.tunnel_url.write().await = new_url;
                    match client.register().await {
                        Ok(new_id) => {
                            *client.node_id.lock().await = Some(new_id);
                            info!("[fabric] Re-registered with new tunnel URL");
                        }
                        Err(e) => {
                            error!("[fabric] Re-registration with new tunnel URL failed: {}", e);
                        }
                    }
                }
            });
        }

        Ok(())
    }
}

/// Start fabric registration if KM_BROKER_URL is set.
/// Returns Ok(()) silently if the env var is not set (standalone mode).
/// If tunnel_url_rx is provided, watches for URL changes and re-registers.
pub async fn maybe_start(tier: SupplyTier, models: Option<std::collections::HashMap<String, f64>>, tunnel_url_rx: Option<watch::Receiver<String>>) -> Result<()> {
    let broker_url = std::env::var("KM_BROKER_URL")
        .unwrap_or_else(|_| "https://km-broker.onrender.com".to_string());

    let node_name = match std::env::var("KM_NODE_NAME") {
        Ok(name) => name,
        Err(_) => {
            anyhow::bail!(
                "KM_NODE_NAME is not set. Set it to the name you registered on the dashboard.\n\
                 Example: KM_NODE_NAME=\"my-node\" km-worker"
            );
        }
    };
    let price: f64 = std::env::var("KM_PRICE")
        .unwrap_or_else(|_| "0.50".to_string())
        .parse()
        .unwrap_or(0.50);
    let tunnel_url = std::env::var("KM_TUNNEL_URL")
        .unwrap_or_else(|_| {
            let port = std::env::var("KM_PORT").unwrap_or_else(|_| "8000".to_string());
            format!("http://127.0.0.1:{}", port)
        });

    let node_secret = std::env::var("KM_NODE_SECRET").ok();

    // Capacity limit env vars
    let token_budget: Option<i64> = std::env::var("KM_TOKEN_BUDGET")
        .ok()
        .and_then(|v| v.parse().ok())
        .filter(|&v| v > 0);
    let budget_window_hours: Option<i32> = std::env::var("KM_BUDGET_WINDOW_HOURS")
        .ok()
        .and_then(|v| v.parse().ok())
        .filter(|&v| v > 0);
    let max_concurrent: Option<i32> = std::env::var("KM_MAX_CONCURRENT")
        .ok()
        .and_then(|v| v.parse().ok())
        .filter(|&v| v > 0);

    if let Some(ref m) = models {
        let model_list: Vec<_> = m.keys().collect();
        info!("[fabric] Registering with broker {} — models: {:?}", broker_url, model_list);
    } else {
        info!("[fabric] Registering with broker {} (tunnel: {})", broker_url, tunnel_url);
    }

    if token_budget.is_some() || max_concurrent.is_some() {
        info!("[fabric] Capacity limits: budget={:?} window={:?}h concurrent={:?}",
            token_budget, budget_window_hours, max_concurrent);
    }

    let client = Arc::new(FabricClient::new(
        broker_url, node_name, tier, price, models, tunnel_url, node_secret,
        token_budget, budget_window_hours, max_concurrent,
    ));
    client.start(tunnel_url_rx).await?;

    Ok(())
}
