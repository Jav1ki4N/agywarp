use crate::clash::model::ProxyMode;
use anyhow::{Context, Result};
use std::time::{Duration, Instant};

#[derive(Debug, Clone, Default)]
pub struct ExitVerification {
    pub is_warp: bool,
    pub ip: String,
    pub loc: String,
    pub colo: String,
    pub endpoint: String,
    pub latency: Duration,
}

pub struct Checker {
    pub trace_url: String,
}

impl Default for Checker {
    fn default() -> Self {
        Self {
            trace_url: "https://1.1.1.1/cdn-cgi/trace".to_string(),
        }
    }
}

impl Checker {
    pub fn new() -> Self {
        Self::default()
    }

    /// Check if local proxy port (e.g. 127.0.0.1:40000) is open and listening
    pub fn is_port_listening(port: u16) -> bool {
        use std::net::{SocketAddr, TcpStream};
        let addr = SocketAddr::from(([127, 0, 0, 1], port));
        TcpStream::connect_timeout(&addr, Duration::from_millis(1000)).is_ok()
    }

    /// Verify outbound IP and WARP status by requesting 1.1.1.1/cdn-cgi/trace via the local proxy
    pub async fn verify_exit(&self, proxy_port: u16, mode: ProxyMode) -> Result<ExitVerification> {
        let proxy_scheme = match mode {
            ProxyMode::Socks5 => "socks5h",
            ProxyMode::Http => "http",
        };
        let proxy_url = format!("{}://127.0.0.1:{}", proxy_scheme, proxy_port);

        let proxy = reqwest::Proxy::all(&proxy_url)
            .with_context(|| format!("Invalid proxy URL: {}", proxy_url))?;

        let client = reqwest::Client::builder()
            .proxy(proxy)
            .timeout(Duration::from_secs(6))
            .danger_accept_invalid_certs(true)
            .build()?;

        let start = Instant::now();
        let resp = client.get(&self.trace_url).send().await
            .with_context(|| format!("Failed to fetch {} via proxy at 127.0.0.1:{}", self.trace_url, proxy_port))?;

        let latency = start.elapsed();
        let text = resp.text().await?;

        let mut verif = parse_trace_response(&text);
        verif.latency = latency;
        Ok(verif)
    }

    /// Probe Google / Gemini APIs via local proxy
    pub async fn probe_google_api(&self, proxy_port: u16, mode: ProxyMode) -> Result<(u16, Duration)> {
        let proxy_scheme = match mode {
            ProxyMode::Socks5 => "socks5h",
            ProxyMode::Http => "http",
        };
        let proxy_url = format!("{}://127.0.0.1:{}", proxy_scheme, proxy_port);
        let proxy = reqwest::Proxy::all(&proxy_url)?;

        let client = reqwest::Client::builder()
            .proxy(proxy)
            .timeout(Duration::from_secs(8))
            .build()?;

        let url = "https://generativelanguage.googleapis.com/v1beta/models";
        let start = Instant::now();
        let resp = client.get(url).send().await?;
        let latency = start.elapsed();
        Ok((resp.status().as_u16(), latency))
    }
}

pub fn parse_trace_response(body: &str) -> ExitVerification {
    let mut verif = ExitVerification::default();
    for line in body.lines() {
        let line = line.trim();
        let mut parts = line.splitn(2, '=');
        let key = parts.next().unwrap_or("").trim();
        let val = parts.next().unwrap_or("").trim();

        match key {
            "warp" => verif.is_warp = val == "on",
            "ip" => verif.ip = val.to_string(),
            "loc" => verif.loc = val.to_string(),
            "colo" => verif.colo = val.to_string(),
            "h" => verif.endpoint = val.to_string(),
            _ => {}
        }
    }
    verif
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_trace() {
        let trace = r#"
fl=123f45
h=1.1.1.1
ip=104.28.192.1
ts=1700000000
visit_scheme=https
uag=Mozilla/5.0
colo=NRT
sliver=none
http=http/2
loc=JP
tls=TLSv1.3
sni=plaintext
warp=on
gateway=off
rbi=off
kex=X25519
"#;
        let v = parse_trace_response(trace);
        assert!(v.is_warp);
        assert_eq!(v.ip, "104.28.192.1");
        assert_eq!(v.loc, "JP");
        assert_eq!(v.colo, "NRT");
        assert_eq!(v.endpoint, "1.1.1.1");
    }
}
