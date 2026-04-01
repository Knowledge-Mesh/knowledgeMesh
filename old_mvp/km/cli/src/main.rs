use anyhow::{bail, Context, Result};
use clap::{Parser, Subcommand};
use serde::{Deserialize, Serialize};

// ── CLI definition ───────────────────────────────────────────────────

#[derive(Parser)]
#[command(name = "mesh", about = "KnowledgeMesh CLI — peer-to-peer AI inference")]
struct Cli {
    /// Broker URL (or set KM_BROKER_URL)
    #[arg(long, env = "KM_BROKER_URL", global = true)]
    broker: Option<String>,

    /// Your identity for tasks and balance (or set KM_NODE_NAME)
    #[arg(long = "as", env = "KM_NODE_NAME", global = true)]
    identity: Option<String>,

    /// Your node secret for authentication (or set KM_NODE_SECRET)
    #[arg(long = "secret", env = "KM_NODE_SECRET", global = true)]
    secret: Option<String>,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Show network overview — online nodes, balances, tasks completed
    Status,

    /// List online nodes with tier, price, and last heartbeat
    Nodes,

    /// Show your credit balance
    Balance,

    /// Submit a task to the network
    Task {
        /// The prompt to send
        prompt: String,

        /// Maximum credits to spend
        #[arg(long, default_value = "5.0")]
        budget: f64,

        /// Prefer a specific tier (api, subscription, local)
        #[arg(long)]
        tier: Option<String>,
    },

    /// Check broker health and connectivity
    Health,

    /// Interactive setup wizard — configure your worker
    Setup,
}

// ── API types ────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct StatusResponse {
    nodes: Vec<NodeInfo>,
    balances: std::collections::HashMap<String, f64>,
    total_tasks_completed: u64,
}

#[derive(Deserialize)]
struct NodeInfo {
    name: String,
    tier: String,
    price_per_million_tokens: f64,
    last_heartbeat: String,
    status: String,
}

#[derive(Deserialize)]
struct HealthResponse {
    status: String,
    uptime_seconds: Option<u64>,
    nodes_online: Option<u64>,
    nodes_total: Option<u64>,
}

#[derive(Serialize)]
struct TaskRequest {
    buyer: String,
    buyer_secret: String,
    messages: Vec<Message>,
    max_budget: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    tier_preference: Option<String>,
}

#[derive(Serialize)]
struct Message {
    role: String,
    content: String,
}

#[derive(Deserialize)]
struct TaskResponse {
    worker_name: String,
    result: TaskResult,
    credits_charged: f64,
}

#[derive(Deserialize)]
struct TaskResult {
    model: String,
    choices: Vec<Choice>,
    usage: Usage,
    tier: String,
}

#[derive(Deserialize)]
struct Choice {
    message: ChoiceMessage,
}

#[derive(Deserialize)]
struct ChoiceMessage {
    content: String,
}

#[derive(Deserialize)]
struct Usage {
    total_tokens: u32,
    token_source: String,
}

#[derive(Deserialize)]
struct ErrorResponse {
    error: String,
}

// ── Main ─────────────────────────────────────────────────────────────

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    if let Err(e) = run(cli).await {
        eprintln!("Error: {}", e);
        std::process::exit(1);
    }
}

async fn run(cli: Cli) -> Result<()> {
    let broker = cli
        .broker
        .as_deref()
        .context("No broker URL. Set --broker or KM_BROKER_URL")?
        .trim_end_matches('/');

    let client = reqwest::Client::new();

    match cli.command {
        Commands::Status => cmd_status(&client, broker).await,
        Commands::Nodes => cmd_nodes(&client, broker).await,
        Commands::Balance => cmd_balance(&client, broker, &cli.identity).await,
        Commands::Task {
            prompt,
            budget,
            tier,
        } => cmd_task(&client, broker, &cli.identity, &cli.secret, &prompt, budget, tier).await,
        Commands::Health => cmd_health(&client, broker).await,
        Commands::Setup => cmd_setup().await,
    }
}

// ── Commands ─────────────────────────────────────────────────────────

async fn cmd_status(client: &reqwest::Client, broker: &str) -> Result<()> {
    let resp: StatusResponse = client
        .get(format!("{}/status", broker))
        .send()
        .await
        .context("Failed to reach broker")?
        .json()
        .await
        .context("Failed to parse broker response")?;

    let online = resp.nodes.iter().filter(|n| n.status == "online").count();

    println!("KnowledgeMesh Network");
    println!("  Nodes: {} online", online);
    println!("  Tasks: {} completed", resp.total_tasks_completed);
    println!();

    if !resp.nodes.is_empty() {
        println!("NODES");
        for n in &resp.nodes {
            let ago = format_ago(&n.last_heartbeat);
            println!(
                "  {:<16} {:<14} ${:.2}/M   {:<8} {}",
                n.name, n.tier, n.price_per_million_tokens, n.status, ago
            );
        }
        println!();
    }

    if !resp.balances.is_empty() {
        println!("BALANCES");
        let mut balances: Vec<_> = resp.balances.iter().collect();
        balances.sort_by(|a, b| a.0.cmp(b.0));
        for (name, amount) in balances {
            println!("  {:<16} {:>10.4} credits", name, amount);
        }
    }

    Ok(())
}

async fn cmd_nodes(client: &reqwest::Client, broker: &str) -> Result<()> {
    let resp: StatusResponse = client
        .get(format!("{}/status", broker))
        .send()
        .await
        .context("Failed to reach broker")?
        .json()
        .await?;

    println!(
        "{:<16} {:<14} {:<10} {:<8} {}",
        "NAME", "TIER", "PRICE", "STATUS", "LAST SEEN"
    );
    for n in &resp.nodes {
        let ago = format_ago(&n.last_heartbeat);
        println!(
            "{:<16} {:<14} ${:.2}/M   {:<8} {}",
            n.name, n.tier, n.price_per_million_tokens, n.status, ago
        );
    }

    Ok(())
}

async fn cmd_balance(
    client: &reqwest::Client,
    broker: &str,
    identity: &Option<String>,
) -> Result<()> {
    let resp: StatusResponse = client
        .get(format!("{}/status", broker))
        .send()
        .await
        .context("Failed to reach broker")?
        .json()
        .await?;

    match identity {
        Some(name) => {
            let balance = resp.balances.get(name.as_str()).copied().unwrap_or(0.0);
            println!("{}: {:.4} credits", name, balance);
        }
        None => {
            let mut balances: Vec<_> = resp.balances.iter().collect();
            balances.sort_by(|a, b| a.0.cmp(b.0));
            for (name, amount) in balances {
                println!("{:<16} {:>10.4} credits", name, amount);
            }
        }
    }

    Ok(())
}

async fn cmd_task(
    client: &reqwest::Client,
    broker: &str,
    identity: &Option<String>,
    secret: &Option<String>,
    prompt: &str,
    budget: f64,
    tier: Option<String>,
) -> Result<()> {
    let buyer = identity
        .as_deref()
        .context("No identity set. Use --as <name> or set KM_NODE_NAME")?;

    let buyer_secret = secret
        .as_deref()
        .context("No secret set. Use --secret <value> or set KM_NODE_SECRET")?;

    let req = TaskRequest {
        buyer: buyer.to_string(),
        buyer_secret: buyer_secret.to_string(),
        messages: vec![Message {
            role: "user".to_string(),
            content: prompt.to_string(),
        }],
        max_budget: budget,
        tier_preference: tier,
    };

    let resp = client
        .post(format!("{}/task", broker))
        .json(&req)
        .send()
        .await
        .context("Failed to reach broker")?;

    let status = resp.status();
    let body = resp.text().await?;

    if !status.is_success() {
        if let Ok(err) = serde_json::from_str::<ErrorResponse>(&body) {
            bail!("{}", err.error);
        }
        bail!("Broker error ({}): {}", status, body);
    }

    let task: TaskResponse =
        serde_json::from_str(&body).context("Failed to parse task response")?;

    println!(
        "Routed to: {} ({})",
        task.worker_name, task.result.tier
    );
    println!("Model:     {}", task.result.model);
    println!(
        "Tokens:    {} ({})",
        task.result.usage.total_tokens, task.result.usage.token_source
    );
    println!("Cost:      {:.4} credits", task.credits_charged);
    println!();

    let text = task
        .result
        .choices
        .first()
        .map(|c| c.message.content.trim())
        .unwrap_or("");
    println!("{}", text);

    Ok(())
}

async fn cmd_health(client: &reqwest::Client, broker: &str) -> Result<()> {
    let resp = client
        .get(format!("{}/health", broker))
        .send()
        .await
        .context("Failed to reach broker — is it running?")?;

    if !resp.status().is_success() {
        bail!("Broker returned {}", resp.status());
    }

    let health: HealthResponse = resp.json().await.context("Failed to parse health response")?;

    println!("Broker: {}", health.status);
    if let Some(uptime) = health.uptime_seconds {
        println!("Uptime: {}", format_duration(uptime));
    }
    if let Some(online) = health.nodes_online {
        let total = health.nodes_total.unwrap_or(online);
        println!("Nodes:  {}/{} online", online, total);
    }

    Ok(())
}

async fn cmd_setup() -> Result<()> {
    use std::io::{self, Write};

    fn prompt(msg: &str) -> String {
        print!("{}", msg);
        io::stdout().flush().unwrap();
        let mut input = String::new();
        io::stdin().read_line(&mut input).unwrap();
        input.trim().to_string()
    }

    println!();
    println!("  KnowledgeMesh Worker Setup");
    println!("  ==========================");
    println!();

    // Node secret
    let secret = prompt("  Your node secret (from dashboard registration): ");
    if secret.is_empty() {
        bail!("Node secret is required. Register at https://km-broker.onrender.com first.");
    }

    // Tier
    println!();
    println!("  Available tiers:");
    println!("    1. api          — I have an Anthropic API key");
    println!("    2. openai       — I have an OpenAI API key");
    println!("    3. subscription — I have a Claude Pro/MAX subscription");
    println!("    4. ollama       — I run local models with Ollama");
    println!("    5. buyer        — I just want to buy (no worker needed)");
    println!();
    let tier_choice = prompt("  Choose tier [1-5]: ");
    let tier = match tier_choice.as_str() {
        "1" | "api" => "api",
        "2" | "openai" => "openai",
        "3" | "subscription" => "subscription",
        "4" | "ollama" | "local" => "ollama",
        "5" | "buyer" => "buyer",
        _ => {
            println!("  Invalid choice, defaulting to 'subscription'");
            "subscription"
        }
    };

    // Credential
    let credential_env = match tier {
        "api" => {
            let key = prompt("  Anthropic API key (sk-ant-...): ");
            if key.is_empty() {
                bail!("API key is required for the api tier.");
            }
            Some(("ANTHROPIC_API_KEY", key))
        }
        "openai" => {
            let key = prompt("  OpenAI API key (sk-...): ");
            if key.is_empty() {
                bail!("API key is required for the openai tier.");
            }
            Some(("OPENAI_API_KEY", key))
        }
        "subscription" => {
            println!();
            println!("  To get your Claude session key:");
            println!("    1. Open claude.ai in Chrome");
            println!("    2. DevTools (F12) > Application > Cookies > claude.ai");
            println!("    3. Copy the 'sessionKey' value");
            println!();
            let key = prompt("  Claude session key: ");
            if key.is_empty() {
                bail!("Session key is required for the subscription tier.");
            }
            Some(("KM_SESSION_KEY", key))
        }
        "ollama" => {
            let model = prompt("  Default Ollama model [llama3.2]: ");
            if !model.is_empty() {
                Some(("OLLAMA_MODEL", model))
            } else {
                None
            }
        }
        _ => None,
    };

    // Generate the startup command
    println!();
    println!("  Setup complete! Start your worker with:");
    println!();
    println!("  export KM_NODE_SECRET={}", secret);
    if let Some((env_name, env_val)) = &credential_env {
        println!("  export {}={}", env_name, env_val);
    }
    if tier != "buyer" {
        println!("  ./km-worker");
    } else {
        println!();
        println!("  As a buyer, you don't need to run a worker.");
        println!("  Use curl or the Python SDK to send tasks:");
        println!();
        println!("  curl -s -X POST https://km-broker.onrender.com/task \\");
        println!("    -H 'Content-Type: application/json' \\");
        println!("    -d '{{\"buyer\":\"YOUR-NAME\",\"buyer_secret\":\"{}\",\"messages\":[{{\"role\":\"user\",\"content\":\"Hello!\"}}]}}'", secret);
    }

    // Write a .env file for convenience
    let write_env = prompt("\n  Write .env file for easy startup? [Y/n]: ");
    if write_env.is_empty() || write_env.to_lowercase().starts_with('y') {
        let mut env_content = format!("KM_NODE_SECRET={}\n", secret);
        if let Some((env_name, env_val)) = &credential_env {
            env_content.push_str(&format!("{}={}\n", env_name, env_val));
        }
        std::fs::write(".env", &env_content)?;
        println!("  Written to .env");
        println!("  Start with: source .env && ./km-worker");
    }

    println!();
    Ok(())
}

// ── Helpers ──────────────────────────────────────────────────────────

fn format_ago(timestamp: &str) -> String {
    let Ok(ts) = chrono::DateTime::parse_from_rfc3339(timestamp) else {
        return timestamp.to_string();
    };
    let now = chrono::Utc::now();
    let diff = now.signed_duration_since(ts.with_timezone(&chrono::Utc));
    let secs = diff.num_seconds();

    if secs < 0 {
        "just now".to_string()
    } else if secs < 60 {
        format!("{}s ago", secs)
    } else if secs < 3600 {
        format!("{}m ago", secs / 60)
    } else if secs < 86400 {
        format!("{}h ago", secs / 3600)
    } else {
        format!("{}d ago", secs / 86400)
    }
}

fn format_duration(secs: u64) -> String {
    if secs < 60 {
        format!("{}s", secs)
    } else if secs < 3600 {
        format!("{}m {}s", secs / 60, secs % 60)
    } else {
        format!("{}h {}m", secs / 3600, (secs % 3600) / 60)
    }
}
