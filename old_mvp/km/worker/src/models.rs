use tracing::{info, warn};

use crate::bridge::BridgeKind;

/// Builds a map of model -> price based on the bridge type.
/// This is sent to the broker during registration so buyers can filter by model.
pub async fn build_models_map(bridge: &BridgeKind) -> Option<std::collections::HashMap<String, f64>> {
    let price: f64 = std::env::var("KM_PRICE")
        .unwrap_or_else(|_| "0.50".to_string())
        .parse()
        .unwrap_or(0.50);

    let mut models = std::collections::HashMap::new();

    match bridge {
        BridgeKind::Anthropic(ref anthropic) => {
            // Dynamically discover available models from Anthropic API
            match anthropic.list_models().await {
                Ok(available) => {
                    info!("[models] Discovered {} Anthropic models", available.len());
                    for model_name in available {
                        let model_price = estimate_model_price(&model_name, price);
                        models.insert(model_name, model_price);
                    }
                }
                Err(e) => {
                    warn!("[models] Could not list Anthropic models: {:#}. Using defaults.", e);
                    models.insert("claude-sonnet-4-20250514".to_string(), price);
                }
            }
        }
        BridgeKind::OpenAI(ref openai) => {
            // Dynamically discover available chat models from OpenAI API
            match openai.list_models().await {
                Ok(available) => {
                    info!("[models] Discovered {} OpenAI chat models", available.len());
                    for model_name in available {
                        let model_price = estimate_model_price(&model_name, price);
                        models.insert(model_name, model_price);
                    }
                }
                Err(e) => {
                    warn!("[models] Could not list OpenAI models: {:#}. Using defaults.", e);
                    models.insert("gpt-4o".to_string(), price);
                }
            }
        }
        BridgeKind::Ollama(ref ollama) => {
            // Discover what models are actually installed
            match ollama.health_check().await {
                Ok(installed) => {
                    for model_name in installed {
                        models.insert(model_name, price);
                    }
                }
                Err(e) => {
                    warn!("[ollama] Could not list models: {:#}. Registering with default.", e);
                    let default_model = std::env::var("OLLAMA_MODEL")
                        .unwrap_or_else(|_| "llama3.2".to_string());
                    models.insert(default_model, price);
                }
            }
        }
        BridgeKind::Session(_) | BridgeKind::Subscription(_) => {
            // Claude subscription — configurable via KM_CLAUDE_MODELS
            let model_list = std::env::var("KM_CLAUDE_MODELS")
                .unwrap_or_else(|_| "claude-sonnet,claude-opus,claude-haiku".to_string());
            for model_name in model_list.split(',').map(|s| s.trim()) {
                if !model_name.is_empty() {
                    let model_price = estimate_model_price(model_name, price);
                    models.insert(model_name.to_string(), model_price);
                }
            }
        }
    }

    if models.is_empty() {
        None
    } else {
        Some(models)
    }
}

/// Estimate price for a model relative to the seller's base price.
/// Uses known API cost ratios so cheaper models are priced proportionally.
pub fn estimate_model_price(model: &str, base_price: f64) -> f64 {
    // API reference costs (blended $/M tokens) — used for relative pricing only
    let (model_ref, base_ref) = if model.contains("claude") || model.contains("anthropic") {
        let model_cost = if model.contains("opus") {
            45.0
        } else if model.contains("haiku") {
            2.5
        } else {
            // sonnet-class is the baseline for Anthropic
            12.0
        };
        (model_cost, 12.0) // base = sonnet
    } else if model.starts_with("gpt") {
        let model_cost = if model.contains("mini") {
            0.375
        } else if model.contains("nano") {
            0.15
        } else if model.starts_with("gpt-4.1") && !model.contains("mini") && !model.contains("nano") {
            10.0
        } else {
            // gpt-4o class is the baseline for OpenAI
            7.5
        };
        (model_cost, 7.5) // base = gpt-4o
    } else {
        return base_price; // unknown model family, use flat price
    };

    // Scale: if haiku is 5x cheaper than sonnet at API, it should be 5x cheaper for the seller too
    base_price * (model_ref / base_ref)
}
