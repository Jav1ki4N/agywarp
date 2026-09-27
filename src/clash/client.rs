use crate::clash::model::{ProxiesResponse, RulesResponse, VersionResponse};
use anyhow::{bail, Context, Result};
use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION, CONTENT_TYPE};
use serde_json::json;
use std::time::Duration;

#[derive(Debug, Clone)]
pub struct MihomoClient {
    base_url: String,
    client: reqwest::Client,
}

impl MihomoClient {
    pub fn new(controller_addr: &str, secret: Option<&str>) -> Result<Self> {
        let mut addr = controller_addr.trim().to_string();
        if !addr.starts_with("http://") && !addr.starts_with("https://") {
            addr = format!("http://{}", addr);
        }

        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
        if let Some(sec) = secret {
            let s = sec.trim();
            if !s.is_empty() {
                let auth_val = format!("Bearer {}", s);
                let header_val = HeaderValue::from_str(&auth_val)
                    .with_context(|| format!("Invalid secret header: {}", s))?;
                headers.insert(AUTHORIZATION, header_val);
            }
        }

        let client = reqwest::Client::builder()
            .default_headers(headers)
            .timeout(Duration::from_secs(10))
            .build()?;

        Ok(Self {
            base_url: addr,
            client,
        })
    }

    pub async fn get_version(&self) -> Result<VersionResponse> {
        let url = format!("{}/version", self.base_url);
        let resp = self.client.get(&url).send().await
            .with_context(|| format!("Failed to connect to Mihomo controller at {}", url))?;

        if !resp.status().is_success() {
            bail!("Mihomo GET /version returned status {}", resp.status());
        }

        let version = resp.json::<VersionResponse>().await?;
        Ok(version)
    }

    pub async fn get_rules(&self) -> Result<RulesResponse> {
        let url = format!("{}/rules", self.base_url);
        let resp = self.client.get(&url).send().await?;
        if !resp.status().is_success() {
            bail!("Mihomo GET /rules returned status {}", resp.status());
        }
        let rules = resp.json::<RulesResponse>().await?;
        Ok(rules)
    }

    pub async fn get_proxies(&self) -> Result<ProxiesResponse> {
        let url = format!("{}/proxies", self.base_url);
        let resp = self.client.get(&url).send().await?;
        if !resp.status().is_success() {
            bail!("Mihomo GET /proxies returned status {}", resp.status());
        }
        let proxies = resp.json::<ProxiesResponse>().await?;
        Ok(proxies)
    }

    /// Load an inline YAML configuration into Mihomo via PUT /configs?force=true
    pub async fn reload_inline_config(&self, yaml_payload: &str) -> Result<()> {
        let url = format!("{}/configs?force=true", self.base_url);
        let body = json!({
            "path": "",
            "payload": yaml_payload,
        });

        let resp = self.client
            .put(&url)
            .json(&body)
            .timeout(Duration::from_secs(20))
            .send()
            .await
            .with_context(|| "Failed to send PUT /configs to Mihomo")?;

        let status = resp.status();
        if !status.is_success() && status != reqwest::StatusCode::NO_CONTENT {
            let msg = resp.text().await.unwrap_or_default();
            bail!("Mihomo config reload failed (HTTP {}): {}", status, msg.trim());
        }

        Ok(())
    }

    /// Reload configuration from a specified file path
    pub async fn reload_file_config(&self, file_path: &str) -> Result<()> {
        let url = format!("{}/configs?force=true", self.base_url);
        let body = json!({
            "path": file_path,
        });

        let resp = self.client
            .put(&url)
            .json(&body)
            .timeout(Duration::from_secs(20))
            .send()
            .await
            .with_context(|| "Failed to reload config from file")?;

        let status = resp.status();
        if !status.is_success() && status != reqwest::StatusCode::NO_CONTENT {
            let msg = resp.text().await.unwrap_or_default();
            bail!("Mihomo config reload from file failed (HTTP {}): {}", status, msg.trim());
        }

        Ok(())
    }
}
