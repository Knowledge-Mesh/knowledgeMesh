mod bridge;
mod config;
mod fabric;
mod handlers;
mod keychain;
mod models;
mod setup;
mod tokens;
mod tunnel;

use std::sync::Arc;

use anyhow::{Context, Result};
use axum::{
    routing::{get, post},
    Router,
};
use tracing::{error, info, warn};

use bridge::{
    anthropic::AnthropicBridge,
    ollama::OllamaBridge,
    openai::OpenAIBridge,
    session::SessionBridge,
    subscription::SubscriptionBridge,
    BridgeKind, SupplyTier,
};

// ── Main ─────────────────────────────────────────────────────────────

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "km_worker=info".into()),
        )
        .init();

    // Step 1: Self-register if KM_INVITE_CODE is set, or load saved config from ~/.km/config.json
    match setup::maybe_self_register().await {
        Ok(true) => {} // config loaded or registered
        Ok(false) => {} // no invite, no saved config — manual mode
        Err(e) => {
            error!("{:#}", e);
            std::process::exit(1);
        }
    }

    // Step 2: If KM_NODE_SECRET is set, fetch config from broker automatically
    let bootstrapped = config::bootstrap_from_broker().await.unwrap_or_else(|e| {
        warn!("[config] Failed to fetch config from broker: {:#}. Using local env vars.", e);
        false
    });
    if bootstrapped {
        info!("[config] Auto-configured from broker");
    }

    let (tier, bridge) = create_bridge()?;

    info!(
        "Starting KnowledgeMesh worker — tier: {}, bridge: {}",
        tier,
        bridge.name()
    );

    // Startup health check — verify credentials work before registering
    info!("[health] Running startup health check...");
    match bridge.health_check().await {
        Ok(()) => info!("[health] Bridge is healthy."),
        Err(e) => {
            error!("[health] Bridge health check failed: {:#}", e);
            error!("[health] Fix your credentials and try again.");
            std::process::exit(1);
        }
    }

    // Build models map based on bridge type
    let models = models::build_models_map(&bridge).await;
    if let Some(ref m) = models {
        info!("Advertising models: {:?}", m.keys().collect::<Vec<_>>());
    }

    let node_secret = std::env::var("KM_NODE_SECRET").ok();
    if node_secret.is_some() {
        info!("[auth] HMAC signature verification enabled for /v1/chat/completions");
    } else {
        warn!("[auth] KM_NODE_SECRET not set — HMAC verification disabled. Requests will be unauthenticated!");
    }

    // Determine max concurrent requests: env var > tier default
    let max_concurrent: usize = std::env::var("KM_MAX_CONCURRENT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or_else(|| match tier {
            SupplyTier::Subscription => 1,
            SupplyTier::Api => 5,
            SupplyTier::Local => 3, // ollama
        });
    info!("[concurrency] Max concurrent requests: {}", max_concurrent);

    let state = Arc::new(handlers::AppState {
        bridge,
        tier,
        node_secret,
        concurrency_semaphore: tokio::sync::Semaphore::new(max_concurrent),
    });

    let app = Router::new()
        .route("/v1/chat/completions", post(handlers::chat_completions))
        .route("/health", get(handlers::health))
        .with_state(state);

    let bind = std::env::var("KM_BIND").unwrap_or_else(|_| "127.0.0.1".to_string());
    let preferred_port: u16 = match std::env::var("KM_PORT") {
        Ok(val) => val.parse().context("KM_PORT must be a valid port number")?,
        Err(_) => find_available_port(&bind).await?,
    };

    let listener = tokio::net::TcpListener::bind(format!("{}:{}", bind, preferred_port))
        .await
        .with_context(|| format!("Failed to bind to {}:{}", bind, preferred_port))?;
    let actual_port = preferred_port;
    let addr = format!("{}:{}", bind, actual_port);

    // Auto-start cloudflared tunnel now that we know the actual port
    let _tunnel_handle = if std::env::var("KM_TUNNEL_URL").is_err()
    {
        info!("Auto-starting cloudflared tunnel for port {}...", actual_port);
        match tunnel::start_tunnel(actual_port).await {
            Ok(handle) => {
                let initial_url = handle.url_rx.borrow().clone();
                std::env::set_var("KM_TUNNEL_URL", &initial_url);
                Some(handle)
            }
            Err(e) => {
                tracing::warn!("Tunnel failed: {:#}. Falling back to local URL.", e);
                None
            }
        }
    } else {
        None
    };

    info!("Listening on {}", addr);

    // Start HTTP server in background, then register with broker.
    // This order matters: the broker's health check needs the server running.
    let _server = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    // Register with broker if KM_BROKER_URL is set (otherwise standalone mode).
    // Pass the tunnel URL watch channel so fabric can re-register on reconnect.
    let tunnel_url_rx = _tunnel_handle.as_ref().map(|h| h.url_rx.clone());
    fabric::maybe_start(tier, models, tunnel_url_rx).await?;

    // Graceful shutdown: deregister from broker on Ctrl+C
    info!("Press Ctrl+C to stop and deregister from the mesh.");
    tokio::signal::ctrl_c().await.ok();
    info!("[shutdown] Ctrl+C received — deregistering from broker...");

    // Deregister from broker
    if let (Ok(broker_url), Ok(node_name), Ok(node_secret)) = (
        std::env::var("KM_BROKER_URL"),
        std::env::var("KM_NODE_NAME"),
        std::env::var("KM_NODE_SECRET"),
    ) {
        let client = reqwest::Client::new();
        let resp = client
            .post(format!("{}/deregister", broker_url))
            .json(&serde_json::json!({
                "name": node_name,
                "node_secret": node_secret
            }))
            .send()
            .await;
        match resp {
            Ok(r) if r.status().is_success() => info!("[shutdown] Deregistered from broker."),
            Ok(r) => warn!("[shutdown] Deregistration response: {}", r.status()),
            Err(e) => warn!("[shutdown] Failed to deregister: {}", e),
        }
    }

    info!("[shutdown] Goodbye.");
    Ok(())
}

// ── Port discovery ──────────────────────────────────────────────────

/// Try port 8000, then 8001 … 8010. Returns the first port that is free.
async fn find_available_port(bind: &str) -> Result<u16> {
    const BASE_PORT: u16 = 8000;
    const MAX_PORT: u16 = 8010;

    for port in BASE_PORT..=MAX_PORT {
        let addr = format!("{}:{}", bind, port);
        match tokio::net::TcpListener::bind(&addr).await {
            Ok(_listener) => {
                // Drop the listener immediately — we just needed to check availability.
                // The caller will bind again on the returned port.
                info!("Auto-selected free port {}", port);
                return Ok(port);
            }
            Err(_) => {
                warn!("Port {} in use, trying next...", port);
            }
        }
    }

    anyhow::bail!(
        "No free port found in range {}-{}. Set KM_PORT to override.",
        BASE_PORT,
        MAX_PORT
    )
}

// ── Bridge creation ──────────────────────────────────────────────────

fn create_bridge() -> Result<(SupplyTier, BridgeKind)> {
    let tier_str = std::env::var("KM_TIER").unwrap_or_else(|_| "api".to_string());

    match tier_str.as_str() {
        "api" => {
            let api_key = std::env::var("ANTHROPIC_API_KEY")
                .or_else(|_| keychain::retrieve("anthropic_api_key"))
                .context(
                    "No API key found. Set ANTHROPIC_API_KEY env var or run `km-worker setup`",
                )?;
            Ok((SupplyTier::Api, BridgeKind::Anthropic(AnthropicBridge::new(api_key))))
        }
        "subscription" => {
            // New: direct session API (no Chrome extension needed)
            let cookie = std::env::var("KM_SESSION_KEY")
                .or_else(|_| keychain::retrieve("claude_session_key"))
                .context(
                    "No session cookie found. Get it from Chrome DevTools:\n\
                     1. Open claude.ai in Chrome\n\
                     2. DevTools (F12) → Application → Cookies → claude.ai\n\
                     3. Copy the 'sessionKey' value\n\
                     4. Set KM_SESSION_KEY=<value>"
                )?;
            Ok((SupplyTier::Subscription, BridgeKind::Session(SessionBridge::new(cookie))))
        }
        "openai" => {
            let api_key = std::env::var("OPENAI_API_KEY")
                .context("No OpenAI API key found. Set OPENAI_API_KEY env var")?;
            Ok((SupplyTier::Api, BridgeKind::OpenAI(OpenAIBridge::new(api_key))))
        }
        "subscription-legacy" => {
            let bridge_url = std::env::var("KM_BRIDGE_URL").ok();
            Ok((SupplyTier::Subscription, BridgeKind::Subscription(SubscriptionBridge::new(bridge_url))))
        }
        "ollama" | "local" => {
            let base_url = std::env::var("OLLAMA_URL").ok()
                .or_else(|| std::env::var("KM_BRIDGE_URL").ok());
            let model = std::env::var("OLLAMA_MODEL").ok();
            let bridge = OllamaBridge::new(base_url, model);
            Ok((SupplyTier::Local, BridgeKind::Ollama(bridge)))
        }
        other => anyhow::bail!(
            "Unknown tier '{}'. Available tiers:\n  \
             api          — Anthropic API key (ANTHROPIC_API_KEY)\n  \
             openai       — OpenAI API key (OPENAI_API_KEY)\n  \
             subscription — Claude Pro/MAX session (KM_SESSION_KEY)\n  \
             ollama       — Local Ollama models (OLLAMA_URL, OLLAMA_MODEL)",
            other
        ),
    }
}
