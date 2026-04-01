use anyhow::{Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};

use super::{InferenceRequest, InferenceResponse, SupplyTier, TokenSource, TokenUsage};

const OPENAI_API_URL: &str = "https://api.openai.com/v1/chat/completions";
const OPENAI_MODELS_URL: &str = "https://api.openai.com/v1/models";
const DEFAULT_MODEL: &str = "gpt-4o";
const DEFAULT_MAX_TOKENS: u32 = 4096;

pub struct OpenAIBridge {
    client: Client,
    api_key: String,
}

impl OpenAIBridge {
    pub fn new(api_key: String) -> Self {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(120))
            .build()
            .expect("failed to build reqwest client");
        Self { client, api_key }
    }

    pub fn name(&self) -> &str {
        "openai-api"
    }

    pub fn api_key_ref(&self) -> &str {
        &self.api_key
    }

    #[allow(dead_code)]
    pub fn tier(&self) -> SupplyTier {
        SupplyTier::Api
    }

    /// List available models from the OpenAI API.
    /// Filters to only chat-capable models (gpt-*).
    pub async fn list_models(&self) -> Result<Vec<String>> {
        let resp = self
            .client
            .get(OPENAI_MODELS_URL)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .send()
            .await
            .context("Failed to reach OpenAI models API")?;

        if !resp.status().is_success() {
            anyhow::bail!("OpenAI models API error: {}", resp.status());
        }

        let body: OpenAIModelsResponse = resp.json().await
            .context("Failed to parse OpenAI models response")?;

        // Filter to chat-completions-capable models only
        // Exclude: TTS, transcribe, realtime, audio (these use different endpoints)
        // Search models are kept — they use the same chat completions endpoint
        let chat_models: Vec<String> = body.data.into_iter()
            .map(|m| m.id)
            .filter(|id| id.starts_with("gpt-4o") || id.starts_with("gpt-4.1") || id.starts_with("gpt-4-"))
            .filter(|id| {
                !id.contains("tts") &&
                !id.contains("transcribe") &&
                !id.contains("realtime") &&
                !id.contains("audio")
            })
            .collect();

        Ok(chat_models)
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        let model = if request.model.is_empty() {
            DEFAULT_MODEL.to_string()
        } else {
            request.model.clone()
        };

        let max_tokens = request.max_tokens.unwrap_or(DEFAULT_MAX_TOKENS);

        // For search models, auto-enable web search if not explicitly provided
        let web_search_options = if request.web_search_options.is_some() {
            request.web_search_options.clone()
        } else if model.contains("search") {
            Some(serde_json::json!({"search_context_size": "medium"}))
        } else {
            None
        };

        let body = OpenAIRequest {
            model: model.clone(),
            max_tokens: Some(max_tokens),
            messages: request
                .messages
                .iter()
                .map(|m| OpenAIMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                })
                .collect(),
            web_search_options,
        };

        let resp = self
            .client
            .post(OPENAI_API_URL)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await
            .context("Failed to reach OpenAI API")?;

        let status = resp.status();
        let resp_text = resp.text().await.context("Failed to read response body")?;

        if !status.is_success() {
            anyhow::bail!("OpenAI API error ({}): {}", status, resp_text);
        }

        let api_resp: OpenAIResponse =
            serde_json::from_str(&resp_text).context("Failed to parse OpenAI response")?;

        let text = api_resp
            .choices
            .first()
            .map(|c| c.message.content.clone())
            .unwrap_or_default();

        let usage = api_resp.usage.unwrap_or(OpenAIUsage {
            prompt_tokens: 0,
            completion_tokens: 0,
            total_tokens: 0,
        });

        Ok(InferenceResponse {
            text,
            model: api_resp.model,
            usage: TokenUsage {
                input_tokens: usage.prompt_tokens,
                output_tokens: usage.completion_tokens,
                source: TokenSource::Exact,
            },
            tier: SupplyTier::Api,
        })
    }
}

// ── OpenAI API types ────────────────────────────────────────────────

#[derive(Serialize)]
struct OpenAIRequest {
    model: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_tokens: Option<u32>,
    messages: Vec<OpenAIMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    web_search_options: Option<serde_json::Value>,
}

#[derive(Serialize)]
struct OpenAIMessage {
    role: String,
    content: String,
}

#[derive(Deserialize)]
struct OpenAIResponse {
    model: String,
    choices: Vec<OpenAIChoice>,
    usage: Option<OpenAIUsage>,
}

#[derive(Deserialize)]
struct OpenAIChoice {
    message: OpenAIResponseMessage,
}

#[derive(Deserialize)]
struct OpenAIResponseMessage {
    content: String,
}

#[derive(Deserialize)]
struct OpenAIUsage {
    prompt_tokens: u32,
    completion_tokens: u32,
    #[allow(dead_code)]
    total_tokens: u32,
}

// ── Models list types ──────────────────────────────────────────────

#[derive(Deserialize)]
struct OpenAIModelsResponse {
    data: Vec<OpenAIModelEntry>,
}

#[derive(Deserialize)]
struct OpenAIModelEntry {
    id: String,
}
