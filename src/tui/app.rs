use crate::checker::{Checker, ExitVerification};
use crate::clash::model::{ProxyMode, VergeProfiles};
use crate::clash::ClashManager;
use crate::process::{Profile, ProcessStore};
use crate::warp::WarpClient;
use anyhow::Result;
use crossterm::{
    event::{self, Event, KeyCode, KeyEventKind, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{backend::CrosstermBackend, Terminal};
use std::io::stdout;
use std::time::{Duration, Instant};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FocusArea {
    NetworkCard,
    ProcessList,
    Console,
}

pub struct App {
    pub focus: FocusArea,
    pub service_active: bool,
    pub proxy_mode: ProxyMode,
    pub warp_port: u16,
    pub warp_installed: bool,
    pub warp_status: String,
    pub current_node: String,
    pub airport_name: String,
    pub exit_info: Option<ExitVerification>,
    pub profiles: Vec<Profile>,
    pub selected_profile_idx: usize,
    pub logs: Vec<String>,
    pub console_scroll: usize,
    pub should_quit: bool,
    pub clash_mgr: ClashManager,
    pub warp_client: WarpClient,
    pub process_store: ProcessStore,
    pub checker: Checker,
}

impl App {
    pub async fn new() -> Result<Self> {
        let clash_mgr = ClashManager::new(None)?;
        let warp_client = WarpClient::new(None);
        let process_store = ProcessStore::new(None)?;
        let checker = Checker::new();

        let profiles = process_store.load_or_init().unwrap_or_default();
        let warp_detect = warp_client.detect();

        let mut app = Self {
            focus: FocusArea::NetworkCard,
            service_active: false,
            proxy_mode: ProxyMode::Socks5,
            warp_port: 40000,
            warp_installed: warp_detect.installed,
            warp_status: "DISCONNECTED".to_string(),
            current_node: "---".to_string(),
            airport_name: "---".to_string(),
            exit_info: None,
            profiles,
            selected_profile_idx: 0,
            logs: Vec::new(),
            console_scroll: 0,
            should_quit: false,
            clash_mgr,
            warp_client,
            process_store,
            checker,
        };

        app.log("INFO", "agywarp TUI initialized");
        app.refresh_status().await;
        Ok(app)
    }

    pub fn log(&mut self, level: &str, msg: &str) {
        let time_str = chrono::Local::now().format("%H:%M:%S").to_string();
        self.logs.push(format!("[{}] [{}] {}", time_str, level, msg));
    }

    pub async fn refresh_status(&mut self) {
        // 1. Inspect Mihomo
        match self.clash_mgr.preflight().await {
            Ok(check) => {
                self.service_active = check.active_session;
                self.log("OK", &format!("Mihomo {} connected (TUN: {})", check.mihomo_version, if check.tun_enabled { "ON" } else { "OFF" }));
            }
            Err(e) => {
                self.log("WARN", &format!("Mihomo controller check: {}", e));
            }
        }

        // 2. Inspect active airport / subscription
        if let Ok(data) = std::fs::read_to_string(self.clash_mgr.profiles_path()) {
            if let Ok(vp) = serde_yaml::from_str::<VergeProfiles>(&data) {
                if let Some(curr) = vp.current {
                    for item in vp.items {
                        if item.uid == curr {
                            self.airport_name = item.name.unwrap_or(item.uid);
                            break;
                        }
                    }
                }
            }
        }

        // 3. Inspect active node selector
        if let Ok(client) = self.clash_mgr.get_client() {
            if let Ok(proxies) = client.get_proxies().await {
                if let Some(global) = proxies.proxies.get("GLOBAL") {
                    if let Some(now) = &global.now {
                        self.current_node = now.clone();
                    }
                } else if let Some((_, item)) = proxies.proxies.iter().find(|(_, p)| p.proxy_type.eq_ignore_ascii_case("Selector")) {
                    if let Some(now) = &item.now {
                        self.current_node = now.clone();
                    }
                }
            }
        }

        // 4. Query WARP Status
        if self.warp_installed {
            if let Ok(st) = self.warp_client.status() {
                self.warp_status = st.status;
                if st.proxy_port > 0 {
                    self.warp_port = st.proxy_port;
                }
            }
        }

        // 5. If service is active, test trace
        if self.service_active {
            if let Ok(exit) = self.checker.verify_exit(self.warp_port, self.proxy_mode).await {
                self.exit_info = Some(exit);
            }
        } else {
            self.exit_info = None;
        }
    }

    pub async fn toggle_service(&mut self) {
        if self.service_active {
            self.log("INFO", "Stopping agywarp routing...");
            match self.clash_mgr.stop_runtime().await {
                Ok(warp_connected_by_us) => {
                    self.service_active = false;
                    self.exit_info = None;
                    if warp_connected_by_us {
                        let _ = self.warp_client.disconnect();
                    }
                    self.log("OK", "Routing stopped, base config restored");
                }
                Err(e) => {
                    self.log("ERROR", &format!("Failed to stop routing: {}", e));
                }
            }
        } else {
            self.log("INFO", "Starting agywarp routing...");
            let mut rules = Vec::new();
            for p in &self.profiles {
                if p.enabled {
                    rules.extend(p.to_process_rules(crate::clash::RUNTIME_PROXY));
                }
            }

            if rules.is_empty() {
                self.log("WARN", "No process rules enabled! Please enable at least one process group.");
                return;
            }

            let mut connected_by_us = false;
            if self.warp_installed {
                if let Ok(st) = self.warp_client.status() {
                    if st.status != "CONNECTED" {
                        self.log("INFO", "Connecting WARP via warp-cli...");
                        if self.warp_client.connect().is_ok() {
                            connected_by_us = true;
                        }
                    }
                }
            }

            match self.clash_mgr.start_runtime(&rules, self.warp_port, self.proxy_mode, connected_by_us).await {
                Ok(count) => {
                    self.service_active = true;
                    self.log("OK", &format!("Routing ON! {} process rules active via WARP", count));
                    if let Ok(exit) = self.checker.verify_exit(self.warp_port, self.proxy_mode).await {
                        self.exit_info = Some(exit);
                    }
                }
                Err(e) => {
                    self.log("ERROR", &format!("Failed to start routing: {}", e));
                }
            }
        }
    }

    pub fn toggle_selected_profile(&mut self) {
        if self.profiles.is_empty() {
            return;
        }
        if self.service_active {
            self.log("WARN", "Cannot edit profiles while routing is ON. Stop service first.");
            return;
        }

        let idx = self.selected_profile_idx;
        self.profiles[idx].enabled = !self.profiles[idx].enabled;
        let _ = self.process_store.save(&self.profiles);
        let name = self.profiles[idx].label.clone();
        let state = if self.profiles[idx].enabled { "ON" } else { "OFF" };
        self.log("INFO", &format!("Profile '{}' toggled {}", name, state));
    }

    pub fn toggle_proxy_mode(&mut self) {
        if self.service_active {
            self.log("WARN", "Cannot change protocol while routing is ON.");
            return;
        }
        self.proxy_mode = match self.proxy_mode {
            ProxyMode::Socks5 => ProxyMode::Http,
            ProxyMode::Http => ProxyMode::Socks5,
        };
        self.log("INFO", &format!("Switched proxy protocol to {}", self.proxy_mode.as_str().to_uppercase()));
    }
}

struct TerminalGuard;

impl TerminalGuard {
    fn new() -> Result<Self> {
        enable_raw_mode()?;
        execute!(stdout(), EnterAlternateScreen)?;
        Ok(Self)
    }
}

impl Drop for TerminalGuard {
    fn drop(&mut self) {
        let _ = disable_raw_mode();
        let _ = execute!(stdout(), LeaveAlternateScreen);
    }
}

pub async fn run_tui() -> Result<()> {
    let _guard = TerminalGuard::new()?;
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout()))?;

    let mut app = App::new().await?;
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(1500);

    loop {
        terminal.draw(|f| crate::tui::ui::draw(f, &mut app))?;

        let timeout = tick_rate.saturating_sub(last_tick.elapsed());
        if event::poll(timeout)? {
            if let Event::Key(key) = event::read()? {
                if key.kind == KeyEventKind::Press {
                    match (key.modifiers, key.code) {
                        (KeyModifiers::CONTROL, KeyCode::Char('c')) | (_, KeyCode::Char('q')) => {
                            if app.service_active {
                                app.log("WARN", "Please stop service (Space) before quitting!");
                            } else {
                                app.should_quit = true;
                            }
                        }
                        (_, KeyCode::Tab) => {
                            app.focus = match app.focus {
                                FocusArea::NetworkCard => FocusArea::ProcessList,
                                FocusArea::ProcessList => FocusArea::Console,
                                FocusArea::Console => FocusArea::NetworkCard,
                            };
                        }
                        (_, KeyCode::BackTab) => {
                            app.focus = match app.focus {
                                FocusArea::NetworkCard => FocusArea::Console,
                                FocusArea::ProcessList => FocusArea::NetworkCard,
                                FocusArea::Console => FocusArea::ProcessList,
                            };
                        }
                        (_, KeyCode::Char(' ')) => {
                            match app.focus {
                                FocusArea::NetworkCard => app.toggle_service().await,
                                FocusArea::ProcessList => app.toggle_selected_profile(),
                                _ => {}
                            }
                        }
                        (_, KeyCode::Char('p')) => {
                            if app.focus == FocusArea::NetworkCard {
                                app.toggle_proxy_mode();
                            }
                        }
                        (_, KeyCode::Char('r')) => {
                            app.refresh_status().await;
                            app.log("INFO", "Status refreshed");
                        }
                        (_, KeyCode::Up) => {
                            match app.focus {
                                FocusArea::ProcessList => {
                                    if app.selected_profile_idx > 0 {
                                        app.selected_profile_idx -= 1;
                                    }
                                }
                                FocusArea::Console => {
                                    if app.console_scroll > 0 {
                                        app.console_scroll -= 1;
                                    }
                                }
                                _ => {}
                            }
                        }
                        (_, KeyCode::Down) => {
                            match app.focus {
                                FocusArea::ProcessList => {
                                    if !app.profiles.is_empty() && app.selected_profile_idx + 1 < app.profiles.len() {
                                        app.selected_profile_idx += 1;
                                    }
                                }
                                FocusArea::Console => {
                                    app.console_scroll += 1;
                                }
                                _ => {}
                            }
                        }
                        _ => {}
                    }
                }
            }
        }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }

        if app.should_quit {
            break;
        }
    }

    Ok(())
}
