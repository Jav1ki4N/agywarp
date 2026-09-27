use crate::process::model::RunningProcess;
use std::collections::HashSet;
use sysinfo::{ProcessRefreshKind, ProcessesToUpdate, System};

pub struct ProcessScanner;

impl ProcessScanner {
    pub fn scan() -> Vec<RunningProcess> {
        let mut sys = System::new();
        sys.refresh_processes_specifics(
            ProcessesToUpdate::All,
            true,
            ProcessRefreshKind::nothing().with_exe(sysinfo::UpdateKind::OnlyIfNotSet).with_cmd(sysinfo::UpdateKind::OnlyIfNotSet),
        );

        let mut seen = HashSet::new();
        let mut result = Vec::new();

        for (pid, proc) in sys.processes() {
            let name = proc.name().to_string_lossy().to_string();
            let exe = proc.exe().map(|p| p.to_string_lossy().to_string());
            let cmd = proc.cmd().iter().map(|s| s.to_string_lossy().to_string()).collect();

            let key = format!("{}:{}", name, exe.as_deref().unwrap_or(""));
            if seen.insert(key) {
                result.push(RunningProcess {
                    pid: pid.as_u32(),
                    name,
                    executable: exe,
                    cmd,
                });
            }
        }

        result.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
        result
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_scan_processes() {
        let procs = ProcessScanner::scan();
        assert!(!procs.is_empty(), "Should find at least some running processes");
    }
}
