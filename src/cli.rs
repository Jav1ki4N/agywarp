use crate::checker::Checker;
use crate::clash::model::ProxyMode;
use crate::clash::ClashManager;
use crate::process::{ProcessScanner, ProcessStore};
use crate::warp::WarpClient;
use anyhow::{bail, Result};
use clap::{Parser, Subcommand};

#[derive(Parser, Debug)]
#[command(
    name = "agywarp",
    author = "Jav1ki4N <i4Nomercyshown@outlook.com>",
    version = "0.1.0",
    about = "Route Antigravity and selected processes through Cloudflare WARP and Mihomo (Clash Verge Rev)"
)]
pub struct Cli {
    #[command(subcommand)]
    pub command: Option<Commands>,
}

#[derive(Subcommand, Debug)]
pub enum Commands {
    /// Show status of Clash Verge, Mihomo, WARP and active routing
    Status,
    /// Start routing enabled processes through WARP
    On {
        #[arg(short, long, default_value = "40000")]
        port: u16,
        #[arg(short, long, default_value = "socks5")]
        mode: String,
    },
    /// Stop routing and restore original Mihomo configuration
    Off,
    /// Recover clean Mihomo configuration after abnormal termination
    Recover,
    /// List actively running system processes
    Procs,
    /// Verify outbound IP and WARP status via 1.1.1.1/cdn-cgi/trace
    Trace {
        #[arg(short, long, default_value = "40000")]
        port: u16,
        #[arg(short, long, default_value = "socks5")]
        mode: String,
    },
    /// Launch TUI Dashboard
    Tui,
}

pub async fn run_cli() -> Result<()> {
    let cli = Cli::parse();
    let cmd = cli.command.unwrap_or(Commands::Tui);

    match cmd {
        Commands::Status => cmd_status().await,
        Commands::On { port, mode } => cmd_on(port, &mode).await,
        Commands::Off => cmd_off().await,
        Commands::Recover => cmd_recover().await,
        Commands::Procs => cmd_procs(),
        Commands::Trace { port, mode } => cmd_trace(port, &mode).await,
        Commands::Tui => crate::tui::run_tui().await,
    }
}

async fn cmd_status() -> Result<()> {
    println!("=== agywarp Status Check ===");

    // 1. Clash Verge & Mihomo
    match ClashManager::new(None) {
        Ok(mgr) => {
            println!("[Clash Verge] Base directory: {}", mgr.base_dir.display());
            match mgr.preflight().await {
                Ok(check) => {
                    println!("[Mihomo] Version: {}", check.mihomo_version);
                    println!("[Mihomo] Controller: {}", check.controller_addr);
                    println!("[Mihomo] TUN Enabled: {}", if check.tun_enabled { "YES" } else { "NO (Required!)" });
                    println!("[Routing] Active Session: {}", if check.active_session { "ON" } else { "OFF" });
                    if check.active_session {
                        println!("[Routing] Active Rules Count: {}", check.injected_rules.len());
                        for r in &check.injected_rules {
                            println!("   -> {}", r);
                        }
                    }
                }
                Err(e) => {
                    println!("[Mihomo] Error connecting to controller: {}", e);
                }
            }
        }
        Err(e) => {
            println!("[Clash Verge] Could not locate configuration: {}", e);
        }
    }

    // 2. WARP
    let warp = WarpClient::new(None);
    let install = warp.detect();
    if install.installed {
        println!("[WARP] Installed: YES (path: {}, version: {})",
            install.path.map(|p| p.display().to_string()).unwrap_or_default(),
            install.version
        );
        match warp.status() {
            Ok(st) => {
                println!("[WARP] Status: {}", st.status);
                println!("[WARP] Protocol: {}", st.protocol);
                println!("[WARP] Mode: {} on port {}", st.mode, st.proxy_port);
            }
            Err(e) => println!("[WARP] Status query failed: {}", e),
        }
    } else {
        println!("[WARP] Installed: NO (warp-cli not found in PATH or standard directories)");
    }

    // 3. Process Store
    if let Ok(store) = ProcessStore::new(None) {
        if let Ok(profiles) = store.load_or_init() {
            println!("[Profiles] Loaded {} groups from {}", profiles.len(), store.path.display());
            for p in &profiles {
                println!("   - [{}] {} (enabled: {})", p.id, p.label, p.enabled);
                for m in &p.matchers {
                    println!("       * {:?}: {}", m.kind, m.pattern);
                }
            }
        }
    }

    Ok(())
}

async fn cmd_on(port: u16, mode_str: &str) -> Result<()> {
    let mode = match mode_str.to_lowercase().as_str() {
        "socks5" | "socks" => ProxyMode::Socks5,
        "http" => ProxyMode::Http,
        other => bail!("Unsupported proxy mode: {} (use 'socks5' or 'http')", other),
    };

    let mgr = ClashManager::new(None)?;
    let check = mgr.preflight().await?;

    if !check.tun_enabled {
        eprintln!("WARNING: TUN mode does not appear enabled in clash-verge.yaml. Process matching may not route correctly.");
    }

    let store = ProcessStore::new(None)?;
    let profiles = store.load_or_init()?;

    let mut rules = Vec::new();
    for p in &profiles {
        if p.enabled {
            rules.extend(p.to_process_rules("AGYWARP-WARP"));
        }
    }

    if rules.is_empty() {
        bail!("No process routing rules generated. Please enable at least one profile.");
    }

    println!("Injecting {} routing rules into Mihomo via {} (WARP port: {})...", rules.len(), mode.as_str(), port);
    for r in &rules {
        println!("  + {}", r);
    }

    // Check if WARP is connected
    let warp = WarpClient::new(None);
    let mut connected_by_us = false;
    if let Ok(st) = warp.status() {
        if st.status != "CONNECTED" {
            println!("WARP is not connected, attempting to connect via warp-cli...");
            if warp.connect().is_ok() {
                connected_by_us = true;
                println!("WARP connected successfully.");
            }
        }
    }

    mgr.start_runtime(&rules, port, mode, connected_by_us).await?;
    println!(">>> SUCCESS: agywarp routing is now ON! <<<");
    println!("Selected processes will now route via Cloudflare WARP.");
    Ok(())
}

async fn cmd_off() -> Result<()> {
    let mgr = ClashManager::new(None)?;
    println!("Stopping agywarp routing and restoring clean Mihomo configuration...");

    let warp_connected_by_us = mgr.stop_runtime().await?;
    if warp_connected_by_us {
        println!("Disconnecting WARP (since it was connected by agywarp)...");
        let warp = WarpClient::new(None);
        let _ = warp.disconnect();
    }

    println!(">>> SUCCESS: agywarp routing stopped, base configuration restored. <<<");
    Ok(())
}

async fn cmd_recover() -> Result<()> {
    let mgr = ClashManager::new(None)?;
    println!("Recovering clean Mihomo configuration...");
    mgr.recover_runtime().await?;
    println!(">>> SUCCESS: Mihomo base config restored and session cleared. <<<");
    Ok(())
}

fn cmd_procs() -> Result<()> {
    println!("Scanning currently running processes...");
    let procs = ProcessScanner::scan();
    println!("Found {} unique processes:", procs.len());
    for p in procs.iter().take(40) {
        println!("  {:>6}  {:<25} {}", p.pid, p.name, p.executable.as_deref().unwrap_or("-"));
    }
    if procs.len() > 40 {
        println!("  ... and {} more processes.", procs.len() - 40);
    }
    Ok(())
}

async fn cmd_trace(port: u16, mode_str: &str) -> Result<()> {
    let mode = match mode_str.to_lowercase().as_str() {
        "socks5" | "socks" => ProxyMode::Socks5,
        "http" => ProxyMode::Http,
        other => bail!("Unsupported proxy mode: {} (use 'socks5' or 'http')", other),
    };

    println!("Testing exit connectivity via 127.0.0.1:{} ({})...", port, mode.as_str());
    let checker = Checker::new();
    match checker.verify_exit(port, mode).await {
        Ok(v) => {
            println!("Trace verification result:");
            println!("  WARP Active: {}", if v.is_warp { "YES (on)" } else { "NO (off)" });
            println!("  Public IP:   {}", v.ip);
            println!("  Location:    {}", v.loc);
            println!("  Colo (DC):   {}", v.colo);
            println!("  Latency:     {:?}", v.latency);
        }
        Err(e) => {
            println!("Trace check failed: {}", e);
            println!("Make sure local WARP proxy is running on port {}.", port);
        }
    }
    Ok(())
}
