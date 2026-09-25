package clash

// VergeProfiles represents ~/.local/share/io.github.clash-verge-rev.clash-verge-rev/profiles.yaml
type VergeProfiles struct {
	Current string             `yaml:"current"`
	Items   []VergeProfileItem `yaml:"items"`
}

// VergeProfileItem represents an item entry in profiles.yaml
type VergeProfileItem struct {
	UID    string              `yaml:"uid"`
	Type   string              `yaml:"type"` // remote, merge, script, rules, proxies
	Name   string              `yaml:"name"`
	File   string              `yaml:"file"`
	Option *VergeProfileOption `yaml:"option,omitempty"`
}

// VergeProfileOption defines enhancement references associated with a profile
type VergeProfileOption struct {
	Merge   string `yaml:"merge,omitempty"`
	Script  string `yaml:"script,omitempty"`
	Rules   string `yaml:"rules,omitempty"`
	Proxies string `yaml:"proxies,omitempty"`
	Groups  string `yaml:"groups,omitempty"`
}

// ProxyExtension represents a proxies enhancement template (e.g. pGNGM2apHpq0.yaml)
type ProxyExtension struct {
	Prepend []ProxyNode `yaml:"prepend"`
	Append  []ProxyNode `yaml:"append"`
}

// ProxyNode represents a proxy definition (e.g. WARP-LOCAL)
type ProxyNode struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	UDP      bool   `yaml:"udp,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// RuleExtension represents a rules enhancement template (e.g. r4uzTpqME3dW.yaml)
type RuleExtension struct {
	Prepend []string `yaml:"prepend"`
	Append  []string `yaml:"append"`
}

// Inspection is the comprehensive diagnostic report of the Clash/Mihomo environment.
type Inspection struct {
	VergeDir            string `json:"verge_dir"`
	Installed           bool   `json:"installed"`
	ActiveProfileUID    string `json:"active_profile_uid"`
	ActiveProfileName   string `json:"active_profile_name"`
	ProxyExtensionFile  string `json:"proxy_extension_file"`
	RuleExtensionFile   string `json:"rule_extension_file"`
	HasWarpLocalProxy   bool   `json:"has_warp_local_proxy"`
	HasWarpSvcGuardRule bool   `json:"has_warp_svc_guard_rule"`
	LiveRulesCount      int    `json:"live_rules_count"`
	WarpRulesCount      int    `json:"warp_rules_count"`
	MihomoVersion       string `json:"mihomo_version"`
	SocketPath          string `json:"socket_path"`
	SocketAvailable     bool   `json:"socket_available"`
	SummaryStatus       string `json:"summary_status"`
}
