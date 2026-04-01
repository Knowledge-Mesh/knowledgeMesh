use std::sync::Arc;

use anyhow::{bail, Context, Result};
use futures::StreamExt;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::{debug, info, warn};

use super::{InferenceRequest, InferenceResponse, SupplyTier, TokenSource, TokenUsage};
use crate::tokens;

const CLAUDE_API_BASE: &str = "https://claude.ai/api";
const USER_AGENT: &str = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

// ── Public API ───────────────────────────────────────────────────────

pub struct SessionBridge {
    client: Client,
    session_cookie: Arc<RwLock<String>>,
    org_id: Arc<RwLock<Option<String>>>,
}

impl SessionBridge {
    #[allow(dead_code)]
    pub async fn cookie_ref(&self) -> String {
        self.session_cookie.read().await.clone()
    }

    /// Verify the session cookie is valid by pinging Claude.ai's organizations endpoint.
    /// This is a free API call that checks authentication without using tokens.
    pub async fn health_check(&self) -> Result<()> {
        let cookie = self.session_cookie.read().await.clone();
        if cookie.is_empty() {
            anyhow::bail!("KM_SESSION_KEY is empty");
        }
        // Actually ping Claude.ai to verify the cookie works
        self.fetch_org_id().await?;
        Ok(())
    }

    pub fn new(session_cookie: String) -> Self {
        // Normalize cookie format
        let cookie = if session_cookie.contains("sessionKey=") {
            session_cookie
        } else {
            format!("sessionKey={}", session_cookie)
        };

        let client = Client::builder()
            .user_agent(USER_AGENT)
            .timeout(std::time::Duration::from_secs(120))
            .build()
            .expect("failed to build reqwest client");

        Self {
            client,
            session_cookie: Arc::new(RwLock::new(cookie)),
            org_id: Arc::new(RwLock::new(None)),
        }
    }

    pub fn name(&self) -> &str {
        "claude-session"
    }

    #[allow(dead_code)]
    pub fn tier(&self) -> SupplyTier {
        SupplyTier::Subscription
    }

    pub async fn run(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        match self.run_inner(&request).await {
            Ok(resp) => Ok(resp),
            Err(e) if is_auth_error(&e) => {
                warn!(
                    "[session] Session key expired or invalid. Get a new one from Claude.ai DevTools. Original error: {}",
                    e
                );
                bail!(
                    "Claude session expired. The seller needs to refresh their session key."
                )
            }
            Err(e) => Err(e),
        }
    }
}

// ── Internal implementation ──────────────────────────────────────────

impl SessionBridge {
    async fn run_inner(&self, request: &InferenceRequest) -> Result<InferenceResponse> {
        // 1. Ensure org_id is cached
        let org_id = self.ensure_org_id().await?;

        // 2. Create a new conversation
        let conv_id = self.create_conversation(&org_id).await?;

        // 3. Send prompt and collect SSE response
        let result = self.send_completion(&org_id, &conv_id, request).await;

        // 4. Delete conversation (best-effort cleanup)
        if let Err(e) = self.delete_conversation(&org_id, &conv_id).await {
            debug!("[session] Failed to delete conversation {}: {}", conv_id, e);
        }

        let (text, model) = result?;

        // If the SSE stream didn't provide a model (or returned the default
        // placeholder), use the model the caller originally requested.
        let model = if (model.is_empty() || model == "claude-subscription") && !request.model.is_empty() {
            request.model.clone()
        } else {
            model
        };

        // Rate-limit cooldown: sleep briefly to avoid back-to-back requests
        // hitting Claude.ai's rate limiter.
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;

        // 5. Estimate tokens (Claude.ai doesn't return usage stats)
        let input_text: String = request
            .messages
            .iter()
            .map(|m| m.content.as_str())
            .collect::<Vec<_>>()
            .join("\n\n");
        let input_tokens = tokens::estimate_tokens(&input_text);
        let output_tokens = tokens::estimate_tokens(&text);

        Ok(InferenceResponse {
            text,
            model,
            usage: TokenUsage {
                input_tokens,
                output_tokens,
                source: TokenSource::Estimated,
            },
            tier: SupplyTier::Subscription,
        })
    }

    // ── API methods ──────────────────────────────────────────────────

    async fn ensure_org_id(&self) -> Result<String> {
        // Fast path: read lock
        {
            let cached = self.org_id.read().await;
            if let Some(ref id) = *cached {
                return Ok(id.clone());
            }
        }

        // Slow path: fetch and cache
        let mut cached = self.org_id.write().await;
        // Double-check after acquiring write lock
        if let Some(ref id) = *cached {
            return Ok(id.clone());
        }

        let id = self.fetch_org_id().await?;
        *cached = Some(id.clone());
        Ok(id)
    }

    async fn fetch_org_id(&self) -> Result<String> {
        let cookie = self.session_cookie.read().await.clone();
        let url = format!("{}/organizations", CLAUDE_API_BASE);

        let resp = self
            .client
            .get(&url)
            .header("Cookie", &cookie)
            .send()
            .await
            .context("Failed to reach Claude.ai organizations endpoint")?;

        let status = resp.status();
        if !status.is_success() {
            let text = resp.text().await.unwrap_or_default();
            bail!("AUTH_ERROR: Claude.ai organizations returned {}: {}", status, text);
        }

        let orgs: Vec<OrgInfo> = resp.json().await.context("Failed to parse organizations response")?;

        let org_id = orgs
            .first()
            .map(|o| o.uuid.clone())
            .context("No organizations found in Claude.ai account")?;

        info!("[session] Resolved org_id: {}", org_id);
        Ok(org_id)
    }

    async fn create_conversation(&self, org_id: &str) -> Result<String> {
        let cookie = self.session_cookie.read().await.clone();
        let url = format!(
            "{}/organizations/{}/chat_conversations",
            CLAUDE_API_BASE, org_id
        );

        let body = CreateConversationBody {
            name: String::new(),
            uuid: uuid::Uuid::new_v4().to_string(),
        };

        let resp = self
            .client
            .post(&url)
            .header("Cookie", &cookie)
            .json(&body)
            .send()
            .await
            .context("Failed to create Claude.ai conversation")?;

        let status = resp.status();
        if !status.is_success() {
            let text = resp.text().await.unwrap_or_default();
            bail!("AUTH_ERROR: Claude.ai create conversation returned {}: {}", status, text);
        }

        let conv: ConversationResponse = resp
            .json()
            .await
            .context("Failed to parse conversation response")?;

        debug!("[session] Created conversation: {}", conv.uuid);
        Ok(conv.uuid)
    }

    async fn send_completion(
        &self,
        org_id: &str,
        conv_id: &str,
        request: &InferenceRequest,
    ) -> Result<(String, String)> {
        let cookie = self.session_cookie.read().await.clone();
        let url = format!(
            "{}/organizations/{}/chat_conversations/{}/completion",
            CLAUDE_API_BASE, org_id, conv_id
        );

        // Concatenate all messages into a single prompt (matches background.js behavior)
        let prompt = request
            .messages
            .iter()
            .map(|m| m.content.as_str())
            .collect::<Vec<_>>()
            .join("\n\n");

        // Map model name to Claude.ai slug if needed
        let model = if request.model.is_empty() {
            None
        } else {
            Some(map_to_claude_slug(&request.model))
        };

        let body = CompletionRequest {
            prompt,
            timezone: "UTC".to_string(),
            model,
            attachments: Vec::new(),
            files: Vec::new(),
        };

        let resp = self
            .client
            .post(&url)
            .header("Cookie", &cookie)
            .json(&body)
            .send()
            .await
            .context("Failed to reach Claude.ai completion endpoint")?;

        let status = resp.status();
        if !status.is_success() {
            let text = resp.text().await.unwrap_or_default();
            if status.as_u16() == 401 || status.as_u16() == 403 {
                bail!("AUTH_ERROR: Claude.ai completion returned {}: {}", status, text);
            }
            bail!("Claude.ai completion error ({}): {}", status, text);
        }

        // Parse the SSE stream
        self.parse_sse_stream(resp).await
    }

    async fn delete_conversation(&self, org_id: &str, conv_id: &str) -> Result<()> {
        let cookie = self.session_cookie.read().await.clone();
        let url = format!(
            "{}/organizations/{}/chat_conversations/{}",
            CLAUDE_API_BASE, org_id, conv_id
        );

        let resp = self
            .client
            .delete(&url)
            .header("Cookie", &cookie)
            .send()
            .await?;

        if !resp.status().is_success() {
            debug!(
                "[session] Delete conversation {} returned {}",
                conv_id,
                resp.status()
            );
        }

        Ok(())
    }

    // ── SSE stream parser ────────────────────────────────────────────

    async fn parse_sse_stream(&self, resp: reqwest::Response) -> Result<(String, String)> {
        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();
        let mut full_text = String::new();
        let mut model = "claude-subscription".to_string();

        while let Some(chunk) = stream.next().await {
            let chunk = chunk.context("Error reading SSE chunk")?;
            buffer.push_str(&String::from_utf8_lossy(&chunk));

            // Process complete lines
            while let Some(newline_pos) = buffer.find('\n') {
                let line = buffer[..newline_pos].to_string();
                buffer = buffer[newline_pos + 1..].to_string();

                let line = line.trim();
                if !line.starts_with("data: ") {
                    continue;
                }

                let json_str = &line[6..];
                if let Ok(event) = serde_json::from_str::<SseEvent>(json_str) {
                    if event.event_type == "completion" {
                        if let Some(ref text) = event.completion {
                            full_text.push_str(text);
                        }
                    }
                    if let Some(ref m) = event.model {
                        model = m.clone();
                    }
                }
                // Silently skip malformed SSE lines (matches background.js behavior)
            }
        }

        if full_text.is_empty() {
            bail!("Empty response from Claude.ai SSE stream");
        }

        Ok((full_text, model))
    }
}

// ── Helper ───────────────────────────────────────────────────────────

fn is_auth_error(err: &anyhow::Error) -> bool {
    err.to_string().contains("AUTH_ERROR")
}

// ── API types ────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct OrgInfo {
    uuid: String,
}

#[derive(Deserialize)]
struct ConversationResponse {
    uuid: String,
}

/// Map user-facing model names to Claude.ai internal slugs.
/// Short names like "claude-sonnet" get mapped to the latest version.
/// Exact slugs or unknown names are passed through as-is.
fn map_to_claude_slug(model: &str) -> String {
    match model {
        "claude-sonnet" => "claude-sonnet-4-6".to_string(),
        "claude-opus" => "claude-opus-4-6".to_string(),
        "claude-haiku" => "claude-haiku-4-5".to_string(),
        other => other.to_string(),
    }
}

#[derive(Serialize)]
struct CreateConversationBody {
    name: String,
    uuid: String,
}

#[derive(Serialize)]
struct CompletionRequest {
    prompt: String,
    timezone: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    model: Option<String>,
    attachments: Vec<()>,
    files: Vec<()>,
}

#[derive(Deserialize)]
struct SseEvent {
    #[serde(rename = "type")]
    event_type: String,
    completion: Option<String>,
    model: Option<String>,
}
