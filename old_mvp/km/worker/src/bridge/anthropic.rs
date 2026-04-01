use anyhow::{Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};

use super::{InferenceRequest, InferenceResponse, SupplyTier, TokenSource, TokenUsage};

const ANTHROPIC_API_URL: &str = "https://api.anthropic.com/v1/messages";
const ANTHROPIC_MODELS_URL: &str = "https://api.anthropic.com/v1/models";
const ANTHROPIC_VERSION: &str = "2023-06-01";
const DEFAULT_MODEL: &str = "claude-sonnet-4-20250514";
const DEFAULT_MAX_TOKENS: u32 = 4096;

pub struct AnthropicBridge {
    client: Client,
    api_key: String,
}

impl AnthropicBridge {
    pub fn new(api_key: String) -> Self {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(120))
            .build()
            .expect("failed to build reqwest client");
        Self { client, api_key }
    }
}

impl AnthropicBridge {
    pub fn name(&self) -> &str {
        "anthropic-api"
    }

    pub fn api_key_ref(&self) -> &str {
        &self.api_key
    }

    /// List available models from the Anthropic API.
    pub async fn list_models(&self) -> Result<Vec<String>> {
        let resp = self
            .client
            .get(ANTHROPIC_MODELS_URL)
            .header("x-api-key", &self.api_key)
            .header("anthropic-version", ANTHROPIC_VERSION)
            .send()
            .await
            .context("Failed to reach Anthropic models API")?;

        if !resp.status().is_success() {
            anyhow::bail!("Anthropic models API error: {}", resp.status());
        }

        let body: ModelsListResponse = resp.json().await
            .context("Failed to parse Anthropic models response")?;

        Ok(body.data.into_iter().map(|m| m.id).collect())
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        let model = if request.model.is_empty() {
            DEFAULT_MODEL.to_string()
        } else {
            request.model.clone()
        };

        let max_tokens = request.max_tokens.unwrap_or(DEFAULT_MAX_TOKENS);

        // When web search is requested, inject the web_search tool
        let tools = if request.web_search_options.is_some() {
            Some(vec![serde_json::json!({
                "type": "web_search_20250305",
                "name": "web_search",
                "max_uses": 5
            })])
        } else {
            None
        };

        let body = AnthropicRequest {
            model: model.clone(),
            max_tokens,
            messages: request
                .messages
                .iter()
                .map(|m| AnthropicMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                })
                .collect(),
            tools,
        };

        let resp = self
            .client
            .post(ANTHROPIC_API_URL)
            .header("x-api-key", &self.api_key)
            .header("anthropic-version", ANTHROPIC_VERSION)
            .header("content-type", "application/json")
            .json(&body)
            .send()
            .await
            .context("Failed to reach Anthropic API")?;

        let status = resp.status();
        let resp_text = resp.text().await.context("Failed to read response body")?;

        if !status.is_success() {
            anyhow::bail!("Anthropic API error ({}): {}", status, resp_text);
        }

        let api_resp: AnthropicResponse =
            serde_json::from_str(&resp_text).context("Failed to parse Anthropic response")?;

        let text = api_resp
            .content
            .iter()
            .filter(|c| c.content_type == "text")
            .map(|c| c.text.as_deref().unwrap_or(""))
            .collect::<Vec<_>>()
            .join("");

        Ok(InferenceResponse {
            text,
            model: api_resp.model,
            usage: TokenUsage {
                input_tokens: api_resp.usage.input_tokens,
                output_tokens: api_resp.usage.output_tokens,
                source: TokenSource::Exact,
            },
            tier: SupplyTier::Api,
        })
    }
}

// ── Anthropic API types ──────────────────────────────────────────────

#[derive(Serialize)]
struct AnthropicRequest {
    model: String,
    max_tokens: u32,
    messages: Vec<AnthropicMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tools: Option<Vec<serde_json::Value>>,
}

#[derive(Serialize)]
struct AnthropicMessage {
    role: String,
    content: String,
}

#[derive(Deserialize)]
struct AnthropicResponse {
    model: String,
    content: Vec<ContentBlock>,
    usage: ApiUsage,
}

#[derive(Deserialize)]
struct ContentBlock {
    #[serde(rename = "type")]
    content_type: String,
    text: Option<String>,
}

#[derive(Deserialize)]
struct ApiUsage {
    input_tokens: u32,
    output_tokens: u32,
}

// ── Models list types ──────────────────────────────────────────────

#[derive(Deserialize)]
struct ModelsListResponse {
    data: Vec<ModelEntry>,
}

#[derive(Deserialize)]
struct ModelEntry {
    id: String,
}
