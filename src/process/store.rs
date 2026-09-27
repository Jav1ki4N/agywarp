use crate::process::model::{default_profiles, Profile};
use anyhow::{Context, Result};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct ProcessStore {
    pub path: PathBuf,
}

impl ProcessStore {
    pub fn new(custom_path: Option<PathBuf>) -> Result<Self> {
        let path = match custom_path {
            Some(p) => p,
            None => default_store_path()?,
        };
        Ok(Self { path })
    }

    pub fn load_or_init(&self) -> Result<Vec<Profile>> {
        if !self.path.exists() {
            let defaults = default_profiles();
            self.save(&defaults)?;
            return Ok(defaults);
        }

        let content = fs::read_to_string(&self.path)
            .with_context(|| format!("Failed to read profiles from {}", self.path.display()))?;

        let profiles: Vec<Profile> = serde_json::from_str(&content)
            .with_context(|| format!("Failed to parse profiles JSON from {}", self.path.display()))?;

        Ok(profiles)
    }

    pub fn save(&self, profiles: &[Profile]) -> Result<()> {
        if let Some(parent) = self.path.parent() {
            fs::create_dir_all(parent)?;
        }

        let tmp_path = self.path.with_extension("tmp");
        let json_str = serde_json::to_string_pretty(profiles)?;
        fs::write(&tmp_path, json_str)?;
        fs::rename(&tmp_path, &self.path)?;
        Ok(())
    }
}

pub fn default_store_path() -> Result<PathBuf> {
    if let Some(cfg) = dirs::config_dir() {
        return Ok(cfg.join("agywarp").join("profiles.json"));
    }
    if let Some(home) = dirs::home_dir() {
        return Ok(home.join(".config").join("agywarp").join("profiles.json"));
    }
    anyhow::bail!("Unable to determine user configuration directory for agywarp")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_store_roundtrip() {
        let temp_dir = std::env::temp_dir().join("agywarp_test_store");
        let store_path = temp_dir.join("profiles.json");
        let _ = fs::remove_dir_all(&temp_dir);

        let store = ProcessStore::new(Some(store_path)).unwrap();
        let loaded = store.load_or_init().unwrap();
        assert_eq!(loaded.len(), 3);
        assert_eq!(loaded[0].label, "Antigravity");

        let _ = fs::remove_dir_all(&temp_dir);
    }
}
