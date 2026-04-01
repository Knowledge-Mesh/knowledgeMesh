use anyhow::{Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};

use super::{InferenceRequest, InferenceResponse, SupplyTier, TokenSource, TokenUsage};

const DEFAULT_OLLAMA_URL: &str = "http://localhost:11434";
const DEFAULT_MODEL: &str = "llama3.2";

pub struct OllamaBridge {
    client: Client,
    base_url: String,
    default_model: String,
}

impl OllamaBridge {
    pub fn new(base_url: Option<String>, model: Option<String>) -> Self {
        Self {
            client: Client::builder()
                .timeout(std::time::Duration::from_secs(120))
                .build()
                .unwrap_or_else(|_| Client::new()),
            base_url: base_url.unwrap_or_else(|| DEFAULT_OLLAMA_URL.to_string()),
            default_model: model.unwrap_or_else(|| DEFAULT_MODEL.to_string()),
        }
    }

    pub fn name(&self) -> &str {
        "ollama"
    }

    #[allow(dead_code)]
    pub fn tier(&self) -> SupplyTier {
        SupplyTier::Local
    }

    /// Check if Ollama is reachable and has at least one model installed.
    pub async fn health_check(&self) -> Result<Vec<String>> {
        let url = format!("{}/api/tags", self.base_url);
        let resp = self.client.get(&url).send().await
            .context("Cannot reach Ollama. Is it running? Start with: ollama serve")?;

        let body: OllamaTagsResponse = resp.json().await
            .context("Failed to parse Ollama tags response")?;

        let models: Vec<String> = body.models.iter().map(|m| m.name.clone()).collect();

        if models.is_empty() {
            anyhow::bail!(
                "Ollama is running but has no models installed.\n\
                 Pull a model first: ollama pull llama3.2"
            );
        }

        tracing::info!("[health] Ollama healthy — {} models: {:?}", models.len(), models);
        Ok(models)
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        let model = if request.model.is_empty() {
            self.default_model.clone()
        } else {
            request.model.clone()
        };

        let body = OllamaChatRequest {
            model: model.clone(),
            messages: request
                .messages
                .iter()
                .map(|m| OllamaMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                })
                .collect(),
            stream: false,
            options: request.temperature.map(|t| OllamaOptions { temperature: Some(t) }),
        };

        let url = format!("{}/api/chat", self.base_url);
        let resp = self
            .client
            .post(&url)
            .json(&body)
            .send()
            .await
            .context("Failed to reach Ollama. Is it running? Start with: ollama serve")?;

        let status = resp.status();
        let resp_text = resp.text().await.context("Failed to read Ollama response")?;

        if !status.is_success() {
            if resp_text.contains("not found") {
                anyhow::bail!(
                    "Model '{}' not found in Ollama. Pull it first: ollama pull {}",
                    model, model
                );
            }
            anyhow::bail!("Ollama error ({}): {}", status, resp_text);
        }

        let api_resp: OllamaChatResponse =
            serde_json::from_str(&resp_text).context("Failed to parse Ollama response")?;

        let text = api_resp.message.content;

        // Ollama returns eval_count (output tokens) and prompt_eval_count (input tokens)
        let input_tokens = api_resp.prompt_eval_count.unwrap_or(0);
        let output_tokens = api_resp.eval_count.unwrap_or(0);

        // If Ollama provides token counts, use them as exact; otherwise estimate
        let (source, in_tok, out_tok) = if input_tokens > 0 || output_tokens > 0 {
            (TokenSource::Exact, input_tokens, output_tokens)
        } else {
            // Fallback: estimate
            let est_in = (request.messages.iter().map(|m| m.content.len()).sum::<usize>() as f64 / 3.8).ceil() as u32;
            let est_out = (text.len() as f64 / 3.8).ceil() as u32;
            (TokenSource::Estimated, est_in, est_out)
        };

        Ok(InferenceResponse {
            text,
            model: api_resp.model,
            usage: TokenUsage {
                input_tokens: in_tok,
                output_tokens: out_tok,
                source,
            },
            tier: SupplyTier::Local,
        })
    }
}

// ── Ollama API types ────────────────────────────────────────────────

#[derive(Serialize)]
struct OllamaChatRequest {
    model: String,
    messages: Vec<OllamaMessage>,
    stream: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    options: Option<OllamaOptions>,
}

#[derive(Serialize)]
struct OllamaMessage {
    role: String,
    content: String,
}

#[derive(Serialize)]
struct OllamaOptions {
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f32>,
}

#[derive(Deserialize)]
struct OllamaChatResponse {
    model: String,
    message: OllamaResponseMessage,
    prompt_eval_count: Option<u32>,
    eval_count: Option<u32>,
}

#[derive(Deserialize)]
struct OllamaResponseMessage {
    content: String,
}

#[derive(Deserialize)]
struct OllamaTagsResponse {
    models: Vec<OllamaModel>,
}

#[derive(Deserialize)]
struct OllamaModel {
    name: String,
}
