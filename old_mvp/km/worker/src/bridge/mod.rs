pub mod anthropic;
pub mod ollama;
pub mod openai;
pub mod session;
pub mod subscription;

use anyhow::Result;
use serde::{Deserialize, Serialize};

use self::anthropic::AnthropicBridge;
use self::ollama::OllamaBridge;
use self::openai::OpenAIBridge;
use self::session::SessionBridge;
use self::subscription::SubscriptionBridge;

/// Enum dispatch for bridge adapters. Each variant wraps a concrete bridge.
/// This avoids dyn-compatibility issues with async trait methods.
#[allow(dead_code)]
pub enum BridgeKind {
    Anthropic(AnthropicBridge),
    OpenAI(OpenAIBridge),
    Ollama(OllamaBridge),
    Subscription(SubscriptionBridge),
    Session(SessionBridge),
}

impl BridgeKind {
    pub fn name(&self) -> &str {
        match self {
            BridgeKind::Anthropic(b) => b.name(),
            BridgeKind::OpenAI(b) => b.name(),
            BridgeKind::Ollama(b) => b.name(),
            BridgeKind::Subscription(b) => b.name(),
            BridgeKind::Session(b) => b.name(),
        }
    }

    #[allow(dead_code)]
    pub fn tier(&self) -> SupplyTier {
        match self {
            BridgeKind::Anthropic(_) => SupplyTier::Api,
            BridgeKind::OpenAI(b) => b.tier(),
            BridgeKind::Ollama(b) => b.tier(),
            BridgeKind::Subscription(b) => b.tier(),
            BridgeKind::Session(b) => b.tier(),
        }
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        match self {
            BridgeKind::Anthropic(b) => b.run(request).await,
            BridgeKind::OpenAI(b) => b.run(request).await,
            BridgeKind::Ollama(b) => b.run(request).await,
            BridgeKind::Subscription(b) => b.run(request).await,
            BridgeKind::Session(b) => b.run(request).await,
        }
    }

    /// Quick health check — verifies credentials are valid without spending money.
    /// For API bridges, hits a lightweight endpoint (models list).
    /// For Ollama, checks connectivity and lists models.
    /// For session bridges, validates cookie format.
    pub async fn health_check(&self) -> Result<()> {
        match self {
            BridgeKind::Anthropic(b) => {
                if b.api_key_ref().is_empty() {
                    anyhow::bail!("ANTHROPIC_API_KEY is empty");
                }
                // Ping Anthropic API to verify the key works (lists models, free call)
                match b.list_models().await {
                    Ok(models) => {
                        tracing::info!("[health] Anthropic API key valid — {} models available", models.len());
                        Ok(())
                    }
                    Err(e) => anyhow::bail!("ANTHROPIC_API_KEY is invalid or expired: {:#}", e),
                }
            }
            BridgeKind::OpenAI(b) => {
                if b.api_key_ref().is_empty() {
                    anyhow::bail!("OPENAI_API_KEY is empty");
                }
                // Ping OpenAI API to verify the key works (lists models, free call)
                match b.list_models().await {
                    Ok(models) => {
                        tracing::info!("[health] OpenAI API key valid — {} chat models available", models.len());
                        Ok(())
                    }
                    Err(e) => anyhow::bail!("OPENAI_API_KEY is invalid or expired: {:#}", e),
                }
            }
            BridgeKind::Ollama(b) => {
                b.health_check().await?;
                Ok(())
            }
            BridgeKind::Session(b) => {
                // Ping Claude.ai to verify the session cookie is valid
                b.health_check().await
            }
            BridgeKind::Subscription(_) => Ok(()),
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum SupplyTier {
    Api,
    Subscription,
    Local,
}

impl std::fmt::Display for SupplyTier {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SupplyTier::Api => write!(f, "api"),
            SupplyTier::Subscription => write!(f, "subscription"),
            SupplyTier::Local => write!(f, "local"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InferenceRequest {
    pub model: String,
    pub messages: Vec<Message>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub web_search_options: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    pub content: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InferenceResponse {
    pub text: String,
    pub model: String,
    pub usage: TokenUsage,
    pub tier: SupplyTier,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TokenUsage {
    pub input_tokens: u32,
    pub output_tokens: u32,
    pub source: TokenSource,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TokenSource {
    Exact,
    Estimated,
}
