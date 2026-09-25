package process

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type MatchKind string

const (
	MatchExecutablePath MatchKind = "executable_path"
	MatchProcessName    MatchKind = "process_name"
	MatchDomain         MatchKind = "domain"
	MatchDomainSuffix   MatchKind = "domain_suffix"
)

type Matcher struct {
	Kind    MatchKind `json:"kind"`
	Pattern string    `json:"pattern"`
}

type Profile struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Enabled  bool      `json:"enabled"`
	Matchers []Matcher `json:"matchers"`
}

type RunningProcess struct {
	PID        int
	Name       string
	Executable string
	Command    []string
}

// GenerateID produces a deterministic 12-char ID from the primary matcher
func GenerateID(kind MatchKind, pattern string) string {
	h := sha256.Sum256([]byte(string(kind) + "\x00" + pattern))
	return hex.EncodeToString(h[:6])
}

// NewProfile creates a new Profile with a deterministic ID
func NewProfile(label string, kind MatchKind, pattern string, enabled bool) Profile {
	return NewGroupProfile(label, []Matcher{{Kind: kind, Pattern: pattern}}, enabled)
}

// NewGroupProfile creates a new Profile with multiple matchers
func NewGroupProfile(label string, matchers []Matcher, enabled bool) Profile {
	primaryPattern := label
	primaryKind := MatchProcessName
	if len(matchers) > 0 {
		primaryPattern = matchers[0].Pattern
		primaryKind = matchers[0].Kind
	}
	return Profile{
		ID:       GenerateID(primaryKind, primaryPattern),
		Label:    label,
		Enabled:  enabled,
		Matchers: matchers,
	}
}

// DefaultProfiles returns process-only routing intents.
func DefaultProfiles() []Profile {
	return []Profile{
		NewProfile("Antigravity", MatchProcessName, "agy", true),
		NewProfile("Google Chrome", MatchProcessName, "chrome", false),
		NewProfile("Gemini CLI", MatchProcessName, "gemini", false),
	}
}

// ToMihomoRules generates Mihomo (Clash Meta) routing rule strings for this profile.
// If the profile is disabled, it returns nil.
func (p Profile) ToMihomoRules(targetProxy string) []string {
	if !p.Enabled {
		return nil
	}
	var rules []string
	for _, m := range p.Matchers {
		switch m.Kind {
		case MatchExecutablePath:
			rules = append(rules, fmt.Sprintf("PROCESS-PATH,%s,%s", m.Pattern, targetProxy))
		case MatchProcessName:
			rules = append(rules, fmt.Sprintf("PROCESS-NAME,%s,%s", m.Pattern, targetProxy))
		case MatchDomain:
			rules = append(rules, fmt.Sprintf("DOMAIN,%s,%s", m.Pattern, targetProxy))
		case MatchDomainSuffix:
			rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", m.Pattern, targetProxy))
		}
	}
	return rules
}

// ToProcessRules compiles only per-process intent. Legacy domain matchers are
// deliberately excluded because they affect traffic from unrelated apps.
func (p Profile) ToProcessRules(targetProxy string) []string {
	if !p.Enabled {
		return nil
	}
	var rules []string
	for _, m := range p.Matchers {
		switch m.Kind {
		case MatchExecutablePath:
			rules = append(rules, fmt.Sprintf("PROCESS-PATH,%s,%s", m.Pattern, targetProxy))
		case MatchProcessName:
			rules = append(rules, fmt.Sprintf("PROCESS-NAME,%s,%s", m.Pattern, targetProxy))
		}
	}
	return rules
}
