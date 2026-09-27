use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum MatchKind {
    ExecutablePath,
    ProcessName,
    Domain,
    DomainSuffix,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Matcher {
    pub kind: MatchKind,
    pub pattern: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Profile {
    pub id: String,
    pub label: String,
    pub enabled: bool,
    pub matchers: Vec<Matcher>,
}

impl Profile {
    pub fn new(label: &str, kind: MatchKind, pattern: &str, enabled: bool) -> Self {
        Self::new_group(label, vec![Matcher { kind, pattern: pattern.to_string() }], enabled)
    }

    pub fn new_group(label: &str, matchers: Vec<Matcher>, enabled: bool) -> Self {
        let (primary_kind, primary_pattern) = if let Some(first) = matchers.first() {
            (first.kind.clone(), first.pattern.clone())
        } else {
            (MatchKind::ProcessName, label.to_string())
        };

        let mut hasher = Sha256::new();
        hasher.update(format!("{:?}\0{}", primary_kind, primary_pattern).as_bytes());
        let hash = hex::encode(hasher.finalize());
        let id = hash[..12].to_string();

        Self {
            id,
            label: label.to_string(),
            enabled,
            matchers,
        }
    }

    /// Compile profile into Mihomo routing rules for the specified proxy target
    pub fn to_process_rules(&self, target_proxy: &str) -> Vec<String> {
        if !self.enabled {
            return Vec::new();
        }

        let mut rules = Vec::new();
        for m in &self.matchers {
            let pat = m.pattern.trim();
            if pat.is_empty() {
                continue;
            }

            match m.kind {
                MatchKind::ProcessName => {
                    rules.push(format!("PROCESS-NAME,{},{}", pat, target_proxy));
                    // On Windows, also add .exe variant if not already present
                    if cfg!(windows) && !pat.to_lowercase().ends_with(".exe") {
                        rules.push(format!("PROCESS-NAME,{}.exe,{}", pat, target_proxy));
                    }
                }
                MatchKind::ExecutablePath => {
                    rules.push(format!("PROCESS-PATH,{},{}", pat, target_proxy));
                }
                // Domains are intentionally excluded from runtime process injection
                _ => {}
            }
        }
        rules
    }
}

pub fn default_profiles() -> Vec<Profile> {
    vec![
        Profile::new_group(
            "Antigravity",
            vec![
                Matcher { kind: MatchKind::ProcessName, pattern: "agy".to_string() },
                Matcher { kind: MatchKind::ProcessName, pattern: "Antigravity".to_string() },
                Matcher { kind: MatchKind::ProcessName, pattern: "antigravity".to_string() },
            ],
            true,
        ),
        Profile::new("Google Chrome", MatchKind::ProcessName, "chrome", false),
        Profile::new("Gemini CLI", MatchKind::ProcessName, "gemini", false),
    ]
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RunningProcess {
    pub pid: u32,
    pub name: String,
    pub executable: Option<String>,
    pub cmd: Vec<String>,
}
