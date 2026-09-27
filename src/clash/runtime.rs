use crate::clash::client::MihomoClient;
use crate::clash::model::{ProxyMode, RuntimeSession, VergeProfiles};
use anyhow::{bail, Context, Result};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;

pub const RUNTIME_PROXY: &str = "AGYWARP-WARP";

#[derive(Debug, Clone)]
pub struct RuntimeCheck {
    pub base_path: PathBuf,
    pub base_hash: String,
    pub controller_addr: String,
    pub secret: Option<String>,
    pub mihomo_version: String,
    pub tun_enabled: bool,
    pub active_session: bool,
    pub injected_rules: Vec<String>,
}

#[derive(Debug, Clone)]
pub struct ClashManager {
    pub base_dir: PathBuf,
}

impl ClashManager {
    pub fn new(custom_base_dir: Option<PathBuf>) -> Result<Self> {
        let base_dir = match custom_base_dir {
            Some(p) => p,
            None => default_base_dir()?,
        };
        Ok(Self { base_dir })
    }

    pub fn base_path(&self) -> PathBuf {
        self.base_dir.join("clash-verge.yaml")
    }

    pub fn session_path(&self) -> PathBuf {
        self.base_dir.join(".agywarp").join("session.json")
    }

    pub fn profiles_path(&self) -> PathBuf {
        self.base_dir.join("profiles.yaml")
    }

    /// Read base clash-verge.yaml and return contents + controller address + secret + tun enabled
    pub fn read_base_info(&self) -> Result<(String, String, Option<String>, bool)> {
        let path = self.base_path();
        if !path.exists() {
            bail!("Base configuration file not found at: {}", path.display());
        }

        let content = fs::read_to_string(&path)
            .with_context(|| format!("Failed to read base config: {}", path.display()))?;

        let doc: serde_yaml::Value = serde_yaml::from_str(&content)
            .with_context(|| "Failed to parse clash-verge.yaml as YAML")?;

        let controller = doc.get("external-controller")
            .and_then(|v| v.as_str())
            .unwrap_or("127.0.0.1:9097")
            .to_string();

        let secret = doc.get("secret")
            .and_then(|v| v.as_str())
            .map(|s| s.to_string());

        let tun_enabled = doc.get("tun")
            .and_then(|v| v.get("enable"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false);

        Ok((content, controller, secret, tun_enabled))
    }

    /// Create MihomoClient based on local configuration
    pub fn get_client(&self) -> Result<MihomoClient> {
        let (_, controller, secret, _) = self.read_base_info()?;
        MihomoClient::new(&controller, secret.as_deref())
    }

    /// Run preflight inspection before modifying configuration
    pub async fn preflight(&self) -> Result<RuntimeCheck> {
        let (content, controller, secret, tun_enabled) = self.read_base_info()?;
        let client = MihomoClient::new(&controller, secret.as_deref())?;

        let ver = client.get_version().await
            .with_context(|| format!("Could not connect to Mihomo controller at {}", controller))?;

        let mihomo_version = ver.version.unwrap_or_else(|| "unknown".to_string());

        let base_hash = hash_content(&content);
        let session = self.load_session()?;

        let mut injected_rules = Vec::new();
        let active_session = session.is_some();
        if let Some(s) = session {
            injected_rules = s.rules;
        } else {
            // Also inspect live rules to see if rules are currently in Mihomo
            if let Ok(rules_resp) = client.get_rules().await {
                for r in rules_resp.rules {
                    if r.proxy == RUNTIME_PROXY {
                        injected_rules.push(format!("{},{},{}", r.rule_type, r.payload, r.proxy));
                    }
                }
            }
        }

        Ok(RuntimeCheck {
            base_path: self.base_path(),
            base_hash,
            controller_addr: controller,
            secret,
            mihomo_version,
            tun_enabled,
            active_session,
            injected_rules,
        })
    }

    /// Build runtime configuration and push it into Mihomo via PUT /configs?force=true
    pub async fn start_runtime(
        &self,
        process_rules: &[String],
        warp_port: u16,
        mode: ProxyMode,
        warp_connected_by_us: bool,
    ) -> Result<usize> {
        let (base_content, controller, secret, _) = self.read_base_info()?;
        let base_hash = hash_content(&base_content);

        // Build the runtime config with injected AGYWARP-WARP and process rules
        let runtime_yaml = build_runtime_config(&base_content, process_rules, warp_port, mode)?;

        let client = MihomoClient::new(&controller, secret.as_deref())?;
        client.reload_inline_config(&runtime_yaml).await?;

        // Query current selectors snapshot
        let mut selectors = HashMap::new();
        let mut current_profile_uid = None;
        if let Ok(proxies_resp) = client.get_proxies().await {
            for (name, item) in proxies_resp.proxies {
                if item.proxy_type.eq_ignore_ascii_case("Selector") {
                    if let Some(now) = item.now {
                        selectors.insert(name, now);
                    }
                }
            }
        }

        if let Ok(profiles_data) = fs::read_to_string(self.profiles_path()) {
            if let Ok(profiles) = serde_yaml::from_str::<VergeProfiles>(&profiles_data) {
                current_profile_uid = profiles.current;
            }
        }

        let session = RuntimeSession {
            proxy_mode: mode,
            base_path: self.base_path().to_string_lossy().to_string(),
            base_hash,
            rules: process_rules.to_vec(),
            warp_connected_by_us,
            profile_uid: current_profile_uid,
            selectors,
        };

        self.save_session(&session)?;
        Ok(process_rules.len())
    }

    /// Stop runtime, restore base config, and return whether WARP was connected by us
    pub async fn stop_runtime(&self) -> Result<bool> {
        let (base_content, controller, secret, _) = self.read_base_info()?;
        let client = MihomoClient::new(&controller, secret.as_deref())?;

        let session = self.load_session()?;
        let warp_connected_by_us = session.as_ref().map(|s| s.warp_connected_by_us).unwrap_or(false);

        // Reload clean base config into Mihomo
        client.reload_inline_config(&base_content).await?;

        // Delete session file
        self.delete_session()?;

        Ok(warp_connected_by_us)
    }

    /// Recover clean Mihomo state if previous session crashed or left dirty rules
    pub async fn recover_runtime(&self) -> Result<()> {
        let (base_content, controller, secret, _) = self.read_base_info()?;
        let client = MihomoClient::new(&controller, secret.as_deref())?;

        client.reload_inline_config(&base_content).await?;
        self.delete_session()?;
        Ok(())
    }

    pub fn load_session(&self) -> Result<Option<RuntimeSession>> {
        let path = self.session_path();
        if !path.exists() {
            return Ok(None);
        }
        let data = fs::read_to_string(&path)?;
        let session = serde_json::from_str::<RuntimeSession>(&data)?;
        Ok(Some(session))
    }

    pub fn save_session(&self, session: &RuntimeSession) -> Result<()> {
        let path = self.session_path();
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        let tmp_path = path.with_extension("tmp");
        let data = serde_json::to_string_pretty(session)?;
        fs::write(&tmp_path, data)?;
        fs::rename(&tmp_path, &path)?;
        Ok(())
    }

    pub fn delete_session(&self) -> Result<()> {
        let path = self.session_path();
        if path.exists() {
            fs::remove_file(&path)?;
        }
        Ok(())
    }
}

pub fn hash_content(s: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(s.as_bytes());
    hex::encode(hasher.finalize())
}

/// Find default Clash Verge directory according to OS conventions
pub fn default_base_dir() -> Result<PathBuf> {
    #[cfg(target_os = "windows")]
    {
        if let Some(appdata) = dirs::config_dir() {
            let p = appdata.join("io.github.clash-verge-rev.clash-verge-rev");
            if p.exists() {
                return Ok(p);
            }
        }
        if let Ok(appdata_env) = std::env::var("APPDATA") {
            let p = PathBuf::from(appdata_env).join("io.github.clash-verge-rev.clash-verge-rev");
            if p.exists() {
                return Ok(p);
            }
        }
    }

    #[cfg(target_os = "linux")]
    {
        if let Some(home) = dirs::home_dir() {
            let p = home.join(".local/share/io.github.clash-verge-rev.clash-verge-rev");
            if p.exists() {
                return Ok(p);
            }
        }
    }

    // Generic fallback for testing or manual installations
    if let Some(home) = dirs::home_dir() {
        let candidates = [
            home.join(".config/clash-verge-rev"),
            home.join(".local/share/io.github.clash-verge-rev.clash-verge-rev"),
        ];
        for c in candidates {
            if c.exists() {
                return Ok(c);
            }
        }
    }

    bail!("Unable to automatically locate Clash Verge Rev configuration directory")
}

/// Build in-memory runtime configuration without altering base config on disk
pub fn build_runtime_config(
    base_yaml: &str,
    process_rules: &[String],
    warp_port: u16,
    mode: ProxyMode,
) -> Result<String> {
    let mut doc: serde_yaml::Value = serde_yaml::from_str(base_yaml)
        .with_context(|| "Base config is not valid YAML")?;

    let root = doc.as_mapping_mut()
        .with_context(|| "Base config must be a YAML mapping")?;

    // 1. Ensure `proxies` list exists and AGYWARP-WARP does not already exist
    let proxies_key = serde_yaml::Value::String("proxies".to_string());
    if !root.contains_key(&proxies_key) {
        root.insert(proxies_key.clone(), serde_yaml::Value::Sequence(Vec::new()));
    }
    let proxies = root.get_mut(&proxies_key).unwrap()
        .as_sequence_mut()
        .with_context(|| "'proxies' must be a YAML sequence")?;

    for p in proxies.iter() {
        if let Some(name) = p.get("name").and_then(|v| v.as_str()) {
            if name == RUNTIME_PROXY {
                bail!("Proxy name {} already exists in base config", RUNTIME_PROXY);
            }
        }
    }

    // Add AGYWARP-WARP proxy entry
    let mut proxy_map = serde_yaml::Mapping::new();
    proxy_map.insert(
        serde_yaml::Value::String("name".to_string()),
        serde_yaml::Value::String(RUNTIME_PROXY.to_string()),
    );
    proxy_map.insert(
        serde_yaml::Value::String("type".to_string()),
        serde_yaml::Value::String(mode.as_str().to_string()),
    );
    proxy_map.insert(
        serde_yaml::Value::String("server".to_string()),
        serde_yaml::Value::String("127.0.0.1".to_string()),
    );
    proxy_map.insert(
        serde_yaml::Value::String("port".to_string()),
        serde_yaml::Value::Number(warp_port.into()),
    );
    proxies.push(serde_yaml::Value::Mapping(proxy_map));

    // 2. Ensure `rules` list exists
    let rules_key = serde_yaml::Value::String("rules".to_string());
    if !root.contains_key(&rules_key) {
        root.insert(rules_key.clone(), serde_yaml::Value::Sequence(Vec::new()));
    }
    let rules = root.get_mut(&rules_key).unwrap()
        .as_sequence_mut()
        .with_context(|| "'rules' must be a YAML sequence")?;

    // 3. Find default outer route target from existing MATCH rule
    let mut outer = "DIRECT".to_string();
    for r in rules.iter() {
        if let Some(rule_str) = r.as_str() {
            if rule_str.starts_with("PROCESS-NAME,warp-svc") {
                bail!("Base config already contains persistent warp-svc rule");
            }
            if rule_str.starts_with("MATCH,") {
                let parts: Vec<&str> = rule_str.split(',').collect();
                if parts.len() >= 2 && !parts[1].trim().is_empty() {
                    outer = parts[1].trim().to_string();
                }
            }
        }
    }

    if outer == RUNTIME_PROXY || outer == "WARP-LOCAL" || outer == "REJECT" || outer == "REJECT-DROP" {
        bail!("Default MATCH route {:?} cannot carry WARP traffic", outer);
    }

    // 4. Construct rules to prepend:
    // Anti-loop guard rules for warp daemon (both Linux warp-svc and Windows warp-svc.exe)
    let mut new_rules: Vec<serde_yaml::Value> = Vec::new();
    new_rules.push(serde_yaml::Value::String(format!("PROCESS-NAME,warp-svc,{}", outer)));
    new_rules.push(serde_yaml::Value::String(format!("PROCESS-NAME,warp-svc.exe,{}", outer)));

    // Process routing rules (e.g. PROCESS-NAME,agy.exe,AGYWARP-WARP)
    let mut seen = std::collections::HashSet::new();
    for pr in process_rules {
        let rule_str = pr.trim();
        if rule_str.is_empty() || !seen.insert(rule_str.to_string()) {
            continue;
        }

        let parts: Vec<&str> = rule_str.split(',').collect();
        if parts.len() != 3 {
            bail!("Invalid process rule format (expected TYPE,PATTERN,TARGET): {}", rule_str);
        }
        if parts[0] != "PROCESS-NAME" && parts[0] != "PROCESS-PATH" {
            bail!("Process rule type must be PROCESS-NAME or PROCESS-PATH: {}", rule_str);
        }
        if parts[2] != RUNTIME_PROXY {
            bail!("Process rule target must be {}: {}", RUNTIME_PROXY, rule_str);
        }

        new_rules.push(serde_yaml::Value::String(rule_str.to_string()));
    }

    // Prepend new rules before original rules
    new_rules.extend(rules.drain(..));
    *rules = new_rules;

    // 5. Ensure `mode: rule`
    let mode_key = serde_yaml::Value::String("mode".to_string());
    root.insert(mode_key, serde_yaml::Value::String("rule".to_string()));

    let output = serde_yaml::to_string(&doc)?;
    Ok(output)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_runtime_config() {
        let base_yaml = r#"
mode: rule
external-controller: 127.0.0.1:9097
proxies:
  - name: "HK Node"
    type: ss
    server: 1.2.3.4
    port: 8388
rules:
  - DOMAIN-SUFFIX,google.com,HK Node
  - MATCH,HK Node
"#;

        let process_rules = vec![
            "PROCESS-NAME,agy.exe,AGYWARP-WARP".to_string(),
            "PROCESS-NAME,chrome.exe,AGYWARP-WARP".to_string(),
        ];

        let result = build_runtime_config(base_yaml, &process_rules, 40000, ProxyMode::Socks5).unwrap();
        assert!(result.contains("AGYWARP-WARP"));
        assert!(result.contains("PROCESS-NAME,warp-svc.exe,HK Node"));
        assert!(result.contains("PROCESS-NAME,agy.exe,AGYWARP-WARP"));
        assert!(result.contains("port: 40000"));
    }

    #[test]
    fn test_reject_persistent_warp_rule() {
        let base_yaml = r#"
rules:
  - PROCESS-NAME,warp-svc,DIRECT
  - MATCH,DIRECT
"#;
        let err = build_runtime_config(base_yaml, &[], 40000, ProxyMode::Socks5).unwrap_err();
        assert!(err.to_string().contains("already contains persistent warp-svc rule"));
    }
}
