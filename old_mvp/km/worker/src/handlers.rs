use std::sync::Arc;

use axum::{
    body::Bytes,
    extract::State,
    http::{HeaderMap, StatusCode},
    response::IntoResponse,
    Json,
};
use hmac::{Hmac, Mac};
use serde::{Deserialize, Serialize};
use sha2::Sha256;
use tracing::{error, warn};

use crate::bridge::{self, BridgeKind, SupplyTier};

// ── App state ────────────────────────────────────────────────────────

pub struct AppState {
    pub bridge: BridgeKind,
    pub tier: SupplyTier,
    pub node_secret: Option<String>,
    pub concurrency_semaphore: tokio::sync::Semaphore,
}

type HmacSha256 = Hmac<Sha256>;

/// Verify the X-KM-Signature header against the HMAC-SHA256 of the request body.
fn verify_hmac_signature(secret: &str, body: &[u8], signature_hex: &str) -> bool {
    let Ok(mut mac) = HmacSha256::new_from_slice(secret.as_bytes()) else {
        return false;
    };
    mac.update(body);
    let Ok(expected) = hex::decode(signature_hex) else {
        return false;
    };
    mac.verify_slice(&expected).is_ok()
}

// ── Handlers ─────────────────────────────────────────────────────────

pub async fn chat_completions(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse {
    // ── HMAC authentication ──────────────────────────────────────────
    if let Some(ref secret) = state.node_secret {
        let signature = match headers.get("X-KM-Signature").and_then(|v| v.to_str().ok()) {
            Some(sig) => sig,
            None => {
                warn!("[auth] Missing X-KM-Signature header");
                return (
                    StatusCode::UNAUTHORIZED,
                    Json(serde_json::json!({ "error": "missing X-KM-Signature header" })),
                )
                    .into_response();
            }
        };

        if !verify_hmac_signature(secret, &body, signature) {
            warn!("[auth] Invalid HMAC signature");
            return (
                StatusCode::UNAUTHORIZED,
                Json(serde_json::json!({ "error": "invalid signature" })),
            )
                .into_response();
        }
    }

    // ── Concurrency control ─────────────────────────────────────────
    let _permit = match state.concurrency_semaphore.try_acquire() {
        Ok(permit) => permit,
        Err(_) => {
            warn!("[concurrency] Too many concurrent requests, rejecting with 429");
            return (
                StatusCode::TOO_MANY_REQUESTS,
                Json(serde_json::json!({ "error": "too many concurrent requests — try again later" })),
            )
                .into_response();
        }
    };

    // ── Deserialize request body ─────────────────────────────────────
    let req: ChatCompletionRequest = match serde_json::from_slice(&body) {
        Ok(r) => r,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({ "error": format!("invalid request body: {}", e) })),
            )
                .into_response();
        }
    };

    let inference_req = bridge::InferenceRequest {
        model: req.model.unwrap_or_default(),
        messages: req
            .messages
            .into_iter()
            .map(|m| bridge::Message {
                role: m.role,
                content: m.content,
            })
            .collect(),
        max_tokens: req.max_tokens,
        temperature: req.temperature,
        web_search_options: req.web_search_options,
    };

    match state.bridge.run(inference_req).await {
        Ok(resp) => {
            let openai_resp = ChatCompletionResponse {
                id: format!("km-{}", uuid::Uuid::new_v4()),
                object: "chat.completion".to_string(),
                model: resp.model,
                choices: vec![Choice {
                    index: 0,
                    message: ResponseMessage {
                        role: "assistant".to_string(),
                        content: resp.text,
                    },
                    finish_reason: "stop".to_string(),
                }],
                usage: UsageResponse {
                    prompt_tokens: resp.usage.input_tokens,
                    completion_tokens: resp.usage.output_tokens,
                    total_tokens: resp.usage.input_tokens + resp.usage.output_tokens,
                    token_source: resp.usage.source,
                },
                tier: state.tier,
            };
            (StatusCode::OK, Json(openai_resp)).into_response()
        }
        Err(e) => {
            error!("Inference failed: {:#}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({ "error": format!("{:#}", e) })),
            )
                .into_response()
        }
    }
}

pub async fn health() -> impl IntoResponse {
    Json(serde_json::json!({ "status": "ok" }))
}

// ── OpenAI-compatible types ──────────────────────────────────────────

#[derive(Deserialize)]
pub struct ChatCompletionRequest {
    model: Option<String>,
    messages: Vec<RequestMessage>,
    max_tokens: Option<u32>,
    temperature: Option<f32>,
    web_search_options: Option<serde_json::Value>,
}

#[derive(Deserialize)]
struct RequestMessage {
    role: String,
    content: String,
}

#[derive(Serialize)]
struct ChatCompletionResponse {
    id: String,
    object: String,
    model: String,
    choices: Vec<Choice>,
    usage: UsageResponse,
    tier: SupplyTier,
}

#[derive(Serialize)]
struct Choice {
    index: u32,
    message: ResponseMessage,
    finish_reason: String,
}

#[derive(Serialize)]
struct ResponseMessage {
    role: String,
    content: String,
}

#[derive(Serialize)]
struct UsageResponse {
    prompt_tokens: u32,
    completion_tokens: u32,
    total_tokens: u32,
    token_source: bridge::TokenSource,
}
