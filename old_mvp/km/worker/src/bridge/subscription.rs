use anyhow::{Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};

use super::{InferenceRequest, InferenceResponse, SupplyTier, TokenSource, TokenUsage};
use crate::tokens;

/// The subscription bridge talks to a local HTTP server run by the Chrome extension.
/// The extension intercepts the Claude.ai web session and exposes it as a simple
/// POST endpoint at localhost. No API key is needed — the extension uses the
/// browser's existing authenticated session.
///
/// Default bridge URL: http://localhost:8100/v1/chat
const DEFAULT_BRIDGE_URL: &str = "http://localhost:8100/v1/chat";

pub struct SubscriptionBridge {
    client: Client,
    bridge_url: String,
}

impl SubscriptionBridge {
    pub fn new(bridge_url: Option<String>) -> Self {
        Self {
            client: Client::new(),
            bridge_url: bridge_url.unwrap_or_else(|| DEFAULT_BRIDGE_URL.to_string()),
        }
    }
}

impl SubscriptionBridge {
    pub fn name(&self) -> &str {
        "anthropic-subscription"
    }

    #[allow(dead_code)]
    pub fn tier(&self) -> SupplyTier {
        SupplyTier::Subscription
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        // Build the request the extension expects
        let body = BridgeRequest {
            messages: request
                .messages
                .iter()
                .map(|m| BridgeMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                })
                .collect(),
            model: if request.model.is_empty() {
                None
            } else {
                Some(request.model.clone())
            },
        };

        let resp = self
            .client
            .post(&self.bridge_url)
            .json(&body)
            .send()
            .await
            .context("Failed to reach subscription bridge — is the Chrome extension running?")?;

        let status = resp.status();
        let resp_text = resp.text().await.context("Failed to read bridge response")?;

        if !status.is_success() {
            anyhow::bail!("Subscription bridge error ({}): {}", status, resp_text);
        }

        let bridge_resp: BridgeResponse =
            serde_json::from_str(&resp_text).context("Failed to parse bridge response")?;

        // Token counting: the browser bridge doesn't give us exact counts.
        // Estimate from character length of prompt + response.
        let input_text: String = request.messages.iter().map(|m| m.content.as_str()).collect();
        let input_tokens = tokens::estimate_tokens(&input_text);
        let output_tokens = tokens::estimate_tokens(&bridge_resp.text);

        Ok(InferenceResponse {
            text: bridge_resp.text,
            model: bridge_resp.model.filter(|m| !m.is_empty()).unwrap_or_else(|| "claude-subscription".to_string()),
            usage: TokenUsage {
                input_tokens,
                output_tokens,
                source: TokenSource::Estimated,
            },
            tier: SupplyTier::Subscription,
        })
    }
}

// ── Bridge protocol types ────────────────────────────────────────────

#[derive(Serialize)]
struct BridgeRequest {
    messages: Vec<BridgeMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    model: Option<String>,
}

#[derive(Serialize)]
struct BridgeMessage {
    role: String,
    content: String,
}

#[derive(Deserialize)]
struct BridgeResponse {
    text: String,
    model: Option<String>,
}
