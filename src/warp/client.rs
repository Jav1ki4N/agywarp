use anyhow::{bail, Result};
use std::path::PathBuf;
use std::process::Command;

#[derive(Debug, Clone, Default)]
pub struct Installation {
    pub installed: bool,
    pub path: Option<PathBuf>,
    pub version: String,
}

#[derive(Debug, Clone, Default)]
pub struct StatusInfo {
    pub status: String,
    pub reason: String,
    pub network_health: String,
    pub protocol: String,
    pub mode: String,
    pub proxy_port: u16,
}

#[derive(Debug, Clone)]
pub struct WarpClient {
    custom_bin: Option<PathBuf>,
}

impl WarpClient {
    pub fn new(custom_bin: Option<PathBuf>) -> Self {
        Self { custom_bin }
    }

    /// Locate warp-cli binary across platforms
    pub fn find_binary(&self) -> Option<PathBuf> {
        if let Some(ref p) = self.custom_bin {
            if p.exists() {
                return Some(p.clone());
            }
        }

        // 1. Check PATH
        let bin_name = if cfg!(windows) { "warp-cli.exe" } else { "warp-cli" };
        if let Ok(path) = which::which(bin_name) {
            return Some(path);
        }

        // 2. Check well-known Windows locations
        #[cfg(target_os = "windows")]
        {
            let candidates = [
                PathBuf::from(r"C:\Program Files\Cloudflare\Cloudflare WARP\warp-cli.exe"),
                PathBuf::from(r"C:\Program Files (x86)\Cloudflare\Cloudflare WARP\warp-cli.exe"),
            ];
            for c in candidates {
                if c.exists() {
                    return Some(c);
                }
            }
            if let Ok(local_appdata) = std::env::var("LOCALAPPDATA") {
                let p = PathBuf::from(local_appdata).join(r"Programs\Cloudflare\warp-cli.exe");
                if p.exists() {
                    return Some(p);
                }
            }
        }

        None
    }

    /// Detect if warp-cli is installed and its version
    pub fn detect(&self) -> Installation {
        let path = match self.find_binary() {
            Some(p) => p,
            None => return Installation::default(),
        };

        let output = Command::new(&path)
            .arg("--version")
            .output();

        match output {
            Ok(out) if out.status.success() => {
                let stdout = String::from_utf8_lossy(&out.stdout);
                let ver = parse_version(&stdout);
                Installation {
                    installed: true,
                    path: Some(path),
                    version: ver,
                }
            }
            _ => Installation {
                installed: true,
                path: Some(path),
                version: "unknown".to_string(),
            },
        }
    }

    /// Query current WARP status and settings
    pub fn status(&self) -> Result<StatusInfo> {
        let path = self.find_binary()
            .ok_or_else(|| anyhow::anyhow!("warp-cli binary not found"))?;

        // 1. Query `warp-cli status`
        let status_out = Command::new(&path)
            .arg("status")
            .output()?;

        let mut info = if status_out.status.success() {
            parse_status_output(&String::from_utf8_lossy(&status_out.stdout))
        } else {
            StatusInfo {
                status: "DISCONNECTED".to_string(),
                ..Default::default()
            }
        };

        // 2. Query `warp-cli settings`
        if let Ok(settings_out) = Command::new(&path).arg("settings").output() {
            if settings_out.status.success() {
                let text = String::from_utf8_lossy(&settings_out.stdout);
                let (proto, mode, port) = parse_settings_output(&text);
                if !proto.is_empty() {
                    info.protocol = proto;
                }
                if !mode.is_empty() {
                    info.mode = mode;
                }
                if port > 0 {
                    info.proxy_port = port;
                }
            }
        }

        Ok(info)
    }

    pub fn connect(&self) -> Result<()> {
        let path = self.find_binary()
            .ok_or_else(|| anyhow::anyhow!("warp-cli binary not found"))?;

        let status = Command::new(&path)
            .arg("connect")
            .status()?;

        if !status.success() {
            bail!("warp-cli connect failed with status {}", status);
        }
        Ok(())
    }

    pub fn disconnect(&self) -> Result<()> {
        let path = self.find_binary()
            .ok_or_else(|| anyhow::anyhow!("warp-cli binary not found"))?;

        let status = Command::new(&path)
            .arg("disconnect")
            .status()?;

        if !status.success() {
            bail!("warp-cli disconnect failed with status {}", status);
        }
        Ok(())
    }
}

pub fn parse_version(output: &str) -> String {
    let line = output.trim();
    let fields: Vec<&str> = line.split_whitespace().collect();
    if fields.len() >= 2 && fields[0] == "warp-cli" {
        fields[1].to_string()
    } else {
        line.to_string()
    }
}

pub fn parse_status_output(output: &str) -> StatusInfo {
    let mut info = StatusInfo {
        status: "DISCONNECTED".to_string(),
        protocol: "---".to_string(),
        mode: "---".to_string(),
        ..Default::default()
    };

    for raw_line in output.lines() {
        let line = raw_line.trim();
        if let Some(val) = line.strip_prefix("Status update:") {
            info.status = val.trim().to_uppercase();
        } else if let Some(val) = line.strip_prefix("Reason:") {
            info.reason = val.trim().to_string();
        } else if let Some(val) = line.strip_prefix("Network:") {
            info.network_health = val.trim().to_string();
        }
    }
    info
}

pub fn parse_settings_output(output: &str) -> (String, String, u16) {
    let mut protocol = String::new();
    let mut mode = String::new();
    let mut port: u16 = 0;

    for raw_line in output.lines() {
        let line = raw_line.trim();

        // WARP tunnel protocol: MASQUE
        if line.to_lowercase().contains("warp tunnel protocol:") {
            if let Some(pos) = line.find(':') {
                protocol = line[pos + 1..].trim().to_string();
            }
        }

        // Mode: WarpProxy on port 40000
        if line.to_lowercase().contains("mode:") {
            if let Some(pos) = line.find(':') {
                let rest = line[pos + 1..].trim();
                if let Some(port_idx) = rest.to_lowercase().find("on port") {
                    mode = rest[..port_idx].trim().to_string();
                    let port_str = rest[port_idx + 7..].trim();
                    if let Ok(p) = port_str.parse::<u16>() {
                        port = p;
                    }
                } else {
                    mode = rest.to_string();
                }
            }
        }
    }

    (protocol, mode, port)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_status() {
        let out = r#"
Status update: Connected
Reason:
Network: healthy
"#;
        let info = parse_status_output(out);
        assert_eq!(info.status, "CONNECTED");
        assert_eq!(info.network_health, "healthy");
    }

    #[test]
    fn test_parse_settings() {
        let out = r#"
Merged configuration:
(consumer overrides)	WARP tunnel protocol: MASQUE
(user set)	Mode: WarpProxy on port 40000
"#;
        let (proto, mode, port) = parse_settings_output(out);
        assert_eq!(proto, "MASQUE");
        assert_eq!(mode, "WarpProxy");
        assert_eq!(port, 40000);
    }
}
