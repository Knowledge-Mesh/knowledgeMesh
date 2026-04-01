use anyhow::{bail, Context, Result};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::watch;
use tokio::time::{timeout, Duration};
use tracing::{debug, error, info, warn};

/// Handle to a running cloudflared tunnel with auto-reconnect.
/// Kills the child process when dropped.
pub struct TunnelHandle {
    /// Receives the current tunnel URL. Updates when tunnel reconnects.
    pub url_rx: watch::Receiver<String>,
    _watchdog_task: tokio::task::JoinHandle<()>,
}

impl Drop for TunnelHandle {
    fn drop(&mut self) {
        info!("[tunnel] Shutting down tunnel watchdog...");
        self._watchdog_task.abort();
    }
}

/// How often to check if the tunnel is still alive (seconds).
const HEALTH_CHECK_INTERVAL_SECS: u64 = 30;

/// Start a Cloudflare Quick Tunnel with auto-reconnect.
///
/// Spawns cloudflared and monitors it. If the process dies OR the tunnel
/// becomes unreachable (silent death), automatically restarts it and
/// broadcasts the new URL via the watch channel.
/// The fabric client listens on this channel to re-register with the broker.
pub async fn start_tunnel(local_port: u16) -> Result<TunnelHandle> {
    // Check that cloudflared is available
    let version_check = Command::new("cloudflared")
        .arg("--version")
        .output()
        .await;

    if version_check.is_err() {
        bail!(
            "cloudflared is not installed.\n\
             Install it with: brew install cloudflared\n\
             Then restart the worker."
        );
    }

    // Start the first tunnel
    let (url, child, drain) = spawn_tunnel(local_port).await?;

    let (url_tx, url_rx) = watch::channel(url.clone());

    // Watchdog: monitors the child process AND checks tunnel health
    let watchdog_task = tokio::spawn(async move {
        let mut current_child = child;
        let mut current_drain = drain;
        let mut current_url = url;

        loop {
            // Race between: process exit vs health check failure
            let needs_restart = tokio::select! {
                // Branch 1: cloudflared process exits
                status = current_child.wait() => {
                    match status {
                        Ok(s) => warn!("[tunnel] cloudflared exited with status {} — reconnecting in 3s...", s),
                        Err(e) => warn!("[tunnel] cloudflared process error: {} — reconnecting in 3s...", e),
                    }
                    true
                }
                // Branch 2: periodic health check
                _ = tunnel_health_loop(&current_url) => {
                    warn!("[tunnel] Tunnel health check failed — restarting tunnel...");
                    // Kill the old cloudflared process
                    let _ = current_child.kill().await;
                    true
                }
            };

            if !needs_restart {
                break;
            }

            current_drain.abort();

            // Retry loop with backoff
            let mut delay = 3;
            loop {
                tokio::time::sleep(Duration::from_secs(delay)).await;

                match spawn_tunnel(local_port).await {
                    Ok((new_url, child, drain)) => {
                        info!("[tunnel] Reconnected with new URL: {}", new_url);
                        // Update the env var so fabric picks it up
                        std::env::set_var("KM_TUNNEL_URL", &new_url);
                        // Broadcast new URL to fabric client
                        let _ = url_tx.send(new_url.clone());
                        current_child = child;
                        current_drain = drain;
                        current_url = new_url;
                        break;
                    }
                    Err(e) => {
                        error!(
                            "[tunnel] Reconnect failed: {} — retrying in {}s...",
                            e, delay
                        );
                        delay = (delay * 2).min(30); // backoff up to 30s
                    }
                }
            }
        }
    });

    Ok(TunnelHandle {
        url_rx,
        _watchdog_task: watchdog_task,
    })
}

/// Periodically pings the tunnel URL. Returns (completes) when the tunnel is dead.
async fn tunnel_health_loop(tunnel_url: &str) {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .unwrap_or_default();

    // Wait a bit before first check — give the tunnel time to stabilize
    tokio::time::sleep(Duration::from_secs(HEALTH_CHECK_INTERVAL_SECS)).await;

    let mut consecutive_failures = 0u32;

    loop {
        // Ping the tunnel URL (just check if it responds — even 404 is fine)
        let check_url = format!("{}/health", tunnel_url);
        match client.get(&check_url).send().await {
            Ok(resp) => {
                let status = resp.status().as_u16();
                // 530 = "origin unregistered from Argo Tunnel" — tunnel is dead
                if status == 530 {
                    warn!("[tunnel] Health check got 530 (tunnel dead)");
                    consecutive_failures += 1;
                } else {
                    if consecutive_failures > 0 {
                        info!("[tunnel] Health check recovered (status {})", status);
                    }
                    consecutive_failures = 0;
                }
            }
            Err(e) => {
                warn!("[tunnel] Health check failed: {}", e);
                consecutive_failures += 1;
            }
        }

        // 3 consecutive failures = tunnel is dead
        if consecutive_failures >= 3 {
            warn!("[tunnel] {} consecutive health check failures — tunnel is dead", consecutive_failures);
            return; // This causes the select! in the watchdog to trigger restart
        }

        tokio::time::sleep(Duration::from_secs(HEALTH_CHECK_INTERVAL_SECS)).await;
    }
}

/// Spawn a single cloudflared process and extract the tunnel URL.
/// Returns (url, child, drain_task).
async fn spawn_tunnel(
    local_port: u16,
) -> Result<(String, Child, tokio::task::JoinHandle<()>)> {
    info!(
        "[tunnel] Starting cloudflared tunnel for 127.0.0.1:{}...",
        local_port
    );

    let mut child = Command::new("cloudflared")
        .arg("tunnel")
        .arg("--url")
        .arg(format!("http://127.0.0.1:{}", local_port))
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .context("Failed to spawn cloudflared")?;

    let stderr = child
        .stderr
        .take()
        .context("Failed to capture cloudflared stderr")?;

    let mut reader = BufReader::new(stderr).lines();

    // Read stderr lines looking for the trycloudflare.com URL.
    let url = timeout(Duration::from_secs(20), async {
        while let Some(line) = reader.next_line().await? {
            debug!("[tunnel] cloudflared: {}", line);

            if let Some(start) = line.find("https://") {
                let candidate = &line[start..];
                let end = candidate
                    .find(|c: char| c.is_whitespace() || c == '|')
                    .unwrap_or(candidate.len());
                let url = &candidate[..end];

                if url.contains("trycloudflare.com") {
                    return Ok::<String, anyhow::Error>(url.to_string());
                }
            }
        }
        bail!("cloudflared exited without providing a tunnel URL")
    })
    .await
    .context("Timed out waiting for cloudflared URL (20s)")?
    .context("Failed to read cloudflared output")?;

    info!("[tunnel] Tunnel ready: {}", url);

    // Drain stderr in background
    let drain_task = tokio::spawn(async move {
        while let Ok(Some(line)) = reader.next_line().await {
            debug!("[tunnel] cloudflared: {}", line);
        }
    });

    Ok((url, child, drain_task))
}
