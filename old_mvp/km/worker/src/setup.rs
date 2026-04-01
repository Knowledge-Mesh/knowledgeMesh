use anyhow::{Context, Result};
use serde::Deserialize;
use tracing::info;

use crate::config::{self, LocalConfig, DEFAULT_BROKER_URL};

#[derive(Deserialize)]
#[allow(dead_code)]
pub struct RegisterUserResponse {
    node_secret: Option<String>,
    name: Option<String>,
    status: Option<String>,
    error: Option<String>,
}

pub async fn self_register(
    broker_url: &str,
    invite_code: &str,
    node_name: &str,
    email: &str,
    tier: &str,
    price: f64,
) -> Result<String> {
    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/register-user", broker_url))
        .json(&serde_json::json!({
            "name": node_name,
            "invite_code": invite_code,
            "email": email,
            "tier": tier,
            "price_per_million_tokens": price,
        }))
        .send()
        .await
        .context("Failed to reach broker for self-registration")?;

    let status = resp.status();
    let body: RegisterUserResponse = resp.json().await
        .context("Failed to parse registration response")?;

    if let Some(error) = body.error {
        anyhow::bail!("Registration failed: {}", error);
    }

    if !status.is_success() {
        anyhow::bail!("Registration failed with status {}", status);
    }

    body.node_secret.ok_or_else(|| anyhow::anyhow!("No node_secret in registration response"))
}

/// Check for KM_INVITE_CODE — if set, self-register and save config.
/// Also loads saved config from ~/.km/config.json if no env vars are set.
pub async fn maybe_self_register() -> Result<bool> {
    // Priority 1: KM_INVITE_CODE means first-time setup
    if let Ok(invite_code) = std::env::var("KM_INVITE_CODE") {
        let broker_url = std::env::var("KM_BROKER_URL")
            .unwrap_or_else(|_| DEFAULT_BROKER_URL.to_string());
        let node_name = std::env::var("KM_NODE_NAME")
            .context("KM_NODE_NAME is required when using KM_INVITE_CODE")?;
        let email = std::env::var("KM_EMAIL")
            .unwrap_or_else(|_| "".to_string());
        let tier = std::env::var("KM_TIER")
            .unwrap_or_else(|_| "ollama".to_string());
        let price: f64 = std::env::var("KM_PRICE")
            .unwrap_or_else(|_| "0.50".to_string())
            .parse()
            .unwrap_or(0.50);

        if email.is_empty() || !email.contains('@') {
            anyhow::bail!("KM_EMAIL is required for registration (used for account recovery)");
        }

        info!("[setup] Registering '{}' with broker {}...", node_name, broker_url);
        let secret = self_register(&broker_url, &invite_code, &node_name, &email, &tier, price).await?;
        info!("[setup] Registered successfully.");

        // Save to local config
        let local_config = LocalConfig {
            node_name: node_name.clone(),
            node_secret: secret.clone(),
            tier: tier.clone(),
            price,
            broker_url: broker_url.clone(),
            email: Some(email),
            token_budget: None,
            budget_window_hours: None,
            max_concurrent: None,
        };
        config::save_local_config(&local_config)?;

        // Set env vars so the rest of the startup flow works
        std::env::set_var("KM_NODE_SECRET", &secret);
        std::env::set_var("KM_NODE_NAME", &node_name);
        std::env::set_var("KM_TIER", &tier);
        std::env::set_var("KM_PRICE", price.to_string());
        std::env::set_var("KM_BROKER_URL", &broker_url);

        return Ok(true);
    }

    // Priority 2: If no KM_NODE_SECRET set, try loading from ~/.km/config.json
    if std::env::var("KM_NODE_SECRET").is_err() {
        if let Some(saved_config) = config::load_local_config() {
            info!("[config] Loaded saved config for '{}'", saved_config.node_name);
            std::env::set_var("KM_NODE_SECRET", &saved_config.node_secret);
            if std::env::var("KM_NODE_NAME").is_err() {
                std::env::set_var("KM_NODE_NAME", &saved_config.node_name);
            }
            if std::env::var("KM_TIER").is_err() {
                std::env::set_var("KM_TIER", &saved_config.tier);
            }
            if std::env::var("KM_PRICE").is_err() {
                std::env::set_var("KM_PRICE", saved_config.price.to_string());
            }
            if std::env::var("KM_BROKER_URL").is_err() {
                std::env::set_var("KM_BROKER_URL", &saved_config.broker_url);
            }
            // Load capacity limits from config if not set via env
            if std::env::var("KM_TOKEN_BUDGET").is_err() {
                if let Some(budget) = saved_config.token_budget {
                    std::env::set_var("KM_TOKEN_BUDGET", budget.to_string());
                }
            }
            if std::env::var("KM_BUDGET_WINDOW_HOURS").is_err() {
                if let Some(window) = saved_config.budget_window_hours {
                    std::env::set_var("KM_BUDGET_WINDOW_HOURS", window.to_string());
                }
            }
            if std::env::var("KM_MAX_CONCURRENT").is_err() {
                if let Some(conc) = saved_config.max_concurrent {
                    std::env::set_var("KM_MAX_CONCURRENT", conc.to_string());
                }
            }
            return Ok(true);
        }
    }

    // Priority 3: If KM_NODE_SECRET or KM_TIER is already set, user is in manual mode
    if std::env::var("KM_NODE_SECRET").is_ok() || std::env::var("KM_TIER").is_ok() {
        return Ok(false);
    }

    // Priority 4: No env vars and no saved config — launch interactive wizard
    return interactive_setup_wizard().await;
}

// ── Interactive setup wizard ─────────────────────────────────────────

fn prompt_line(prompt: &str) -> String {
    use std::io::Write;
    print!("{}", prompt);
    std::io::stdout().flush().ok();
    let mut buf = String::new();
    std::io::stdin().read_line(&mut buf).unwrap_or_default();
    buf.trim().to_string()
}

async fn interactive_setup_wizard() -> Result<bool> {
    println!("Welcome to KnowledgeMesh! Let's set up your worker.\n");

    // 1. Invite code
    let invite_code = prompt_line("Invite code: ");
    if invite_code.is_empty() {
        anyhow::bail!("Invite code is required to register.");
    }

    // 2. Node name
    let node_name = prompt_line("Node name: ");
    if node_name.is_empty() {
        anyhow::bail!("Node name is required.");
    }

    // 3. Email
    let email = prompt_line("Email (for account recovery): ");
    if email.is_empty() || !email.contains('@') {
        anyhow::bail!("A valid email address is required for registration.");
    }

    // 4. Tier
    println!("\nSelect your tier:");
    println!("  1) ollama        — Local Ollama models (free, no API key)");
    println!("  2) api           — Anthropic API key");
    println!("  3) openai        — OpenAI API key");
    println!("  4) subscription  — Claude Pro/MAX session key");
    let tier_choice = prompt_line("Tier [1-4]: ");
    let tier = match tier_choice.as_str() {
        "1" => "ollama",
        "2" => "api",
        "3" => "openai",
        "4" => "subscription",
        _ => anyhow::bail!("Invalid tier choice '{}'. Please enter 1, 2, 3, or 4.", tier_choice),
    };

    // 5. Price with suggested default per tier
    let default_price = match tier {
        "ollama" => "0.10",
        "api" => "0.50",
        "openai" => "0.50",
        "subscription" => "1.00",
        _ => "0.50",
    };
    let price_input = prompt_line(&format!("Price per million tokens [default: {}]: ", default_price));
    let price: f64 = if price_input.is_empty() {
        default_price.parse().unwrap()
    } else {
        price_input.parse().unwrap_or_else(|_| {
            eprintln!("Invalid price, using default {}.", default_price);
            default_price.parse().unwrap()
        })
    };

    // 6. Credential prompt based on tier
    match tier {
        "api" => {
            let key = prompt_line("ANTHROPIC_API_KEY: ");
            if key.is_empty() {
                anyhow::bail!("Anthropic API key is required for the 'api' tier.");
            }
            std::env::set_var("ANTHROPIC_API_KEY", &key);
        }
        "openai" => {
            let key = prompt_line("OPENAI_API_KEY: ");
            if key.is_empty() {
                anyhow::bail!("OpenAI API key is required for the 'openai' tier.");
            }
            std::env::set_var("OPENAI_API_KEY", &key);
        }
        "subscription" => {
            let key = prompt_line("KM_SESSION_KEY (from claude.ai cookies): ");
            if key.is_empty() {
                anyhow::bail!("Session key is required for the 'subscription' tier.");
            }
            std::env::set_var("KM_SESSION_KEY", &key);
        }
        _ => {} // ollama: no credential needed
    }

    // 7. Register with broker
    let broker_url = DEFAULT_BROKER_URL.to_string();
    println!("\nRegistering '{}' with broker {}...", node_name, broker_url);
    let secret = self_register(&broker_url, &invite_code, &node_name, &email, tier, price).await?;
    println!("Registered successfully!");

    // 7b. Optional: capacity limits
    println!("\nOptional capacity limits (press Enter to skip):");
    let token_budget_input = prompt_line("Token budget (max tokens in window, 0=unlimited): ");
    let token_budget: Option<i64> = token_budget_input.parse().ok().filter(|&v: &i64| v > 0);
    let budget_window_input = prompt_line("Budget window hours (e.g. 24, 0=unlimited): ");
    let budget_window_hours: Option<i32> = budget_window_input.parse().ok().filter(|&v: &i32| v > 0);
    let max_concurrent_input = prompt_line("Max concurrent requests (0=tier default): ");
    let max_concurrent: Option<i32> = max_concurrent_input.parse().ok().filter(|&v: &i32| v > 0);

    // 8. Save config to ~/.km/configs/{node_name}.json
    let local_config = LocalConfig {
        node_name: node_name.clone(),
        node_secret: secret.clone(),
        tier: tier.to_string(),
        price,
        broker_url: broker_url.clone(),
        email: Some(email),
        token_budget,
        budget_window_hours,
        max_concurrent,
    };
    config::save_local_config(&local_config)?;

    // 9. Set env vars so the rest of startup works
    std::env::set_var("KM_NODE_SECRET", &secret);
    std::env::set_var("KM_NODE_NAME", &node_name);
    std::env::set_var("KM_TIER", tier);
    std::env::set_var("KM_PRICE", price.to_string());
    std::env::set_var("KM_BROKER_URL", &broker_url);

    Ok(true)
}
