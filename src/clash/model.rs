use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Mode for the local WARP proxy adapter (SOCKS5 or HTTP CONNECT).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum ProxyMode {
    #[default]
    Socks5,
    Http,
}

impl ProxyMode {
    pub fn as_str(&self) -> &'static str {
        match self {
            ProxyMode::Socks5 => "socks5",
            ProxyMode::Http => "http",
        }
    }
}

/// Mihomo version response (/version)
#[derive(Debug, Clone, Deserialize)]
pub struct VersionResponse {
    pub version: Option<String>,
    pub meta: Option<bool>,
}

/// Mihomo rule entry in /rules
#[derive(Debug, Clone, Deserialize)]
pub struct RuleItem {
    #[serde(rename = "type")]
    pub rule_type: String,
    pub payload: String,
    pub proxy: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RulesResponse {
    pub rules: Vec<RuleItem>,
}

/// Mihomo proxy entry in /proxies
#[derive(Debug, Clone, Deserialize)]
pub struct ProxyItem {
    pub name: Option<String>,
    #[serde(rename = "type")]
    pub proxy_type: String,
    pub now: Option<String>,
    #[serde(rename = "provider-name")]
    pub provider_name: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ProxiesResponse {
    pub proxies: HashMap<String, ProxyItem>,
}

/// Clash Verge profiles.yaml structure
#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct VergeProfiles {
    pub current: Option<String>,
    #[serde(default)]
    pub items: Vec<VergeProfileItem>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct VergeProfileItem {
    pub uid: String,
    pub name: Option<String>,
    #[serde(rename = "type")]
    pub item_type: Option<String>,
    pub file: Option<String>,
}

/// agywarp runtime session stored in <base_dir>/.agywarp/session.json
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeSession {
    pub proxy_mode: ProxyMode,
    pub base_path: String,
    pub base_hash: String,
    pub rules: Vec<String>,
    pub warp_connected_by_us: bool,
    #[serde(default)]
    pub profile_uid: Option<String>,
    #[serde(default)]
    pub selectors: HashMap<String, String>,
}
