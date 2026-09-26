package clash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OuterRoute separates the configured selection from observed WARP UDP flows.
// A controller observation proves Mihomo handled the flow, not the remote VPS's identity.
type OuterRoute struct {
	SelectedNode     string
	SelectedProvider string
	Target           string
	Node             string
	Provider         string
	Airport          string
	Status           string
	Detail           string
	Chains           []string
}

type OuterRouteInspector interface {
	InspectOuterRoute(context.Context) (OuterRoute, error)
}

type routeProxy struct {
	Type     string
	Now      string
	Provider string `json:"provider-name"`
}

type routeConnection struct {
	Metadata struct {
		Process     string
		ProcessPath string
		Network     string
	}
	Chains         []string
	ProviderChains []string
	Rule           string
	RulePayload    string
}

func (m *SystemdManager) InspectOuterRoute(ctx context.Context) (OuterRoute, error) {
	r := OuterRoute{Status: "UNOBSERVED", Node: "---", Provider: "---", Airport: "---", Target: "DIRECT"}
	base, err := os.ReadFile(m.basePath())
	if err != nil {
		return r, err
	}
	_, root, err := configNode(base)
	if err != nil {
		return r, err
	}
	if rules := field(root, "rules"); rules != nil {
		for _, rule := range rules.Content {
			parts := strings.Split(rule.Value, ",")
			if len(parts) >= 2 && strings.TrimSpace(parts[0]) == "MATCH" {
				r.Target = strings.TrimSpace(parts[1])
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(m.BaseDir, "profiles.yaml"))
	if err != nil {
		return r, err
	}
	var profiles VergeProfiles
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return r, err
	}
	for _, profile := range profiles.Items {
		if profile.UID == profiles.Current {
			r.Airport = profile.Name
		}
	}
	var proxies struct{ Proxies map[string]routeProxy }
	if err := m.getJSON(ctx, "/proxies", &proxies); err != nil {
		return r, err
	}
	node := r.Target
	seen := map[string]bool{}
	for {
		if seen[node] {
			return r, fmt.Errorf("proxy selection cycle at %s", node)
		}
		seen[node] = true
		p, ok := proxies.Proxies[node]
		if !ok || p.Now == "" {
			break
		}
		node = p.Now
	}
	r.Node = node
	if p, ok := proxies.Proxies[node]; ok {
		r.Provider = p.Provider
		if r.Provider == "" {
			r.Provider = "---"
		}
		switch strings.ToLower(p.Type) {
		case "selector", "urltest", "fallback", "loadbalance", "relay":
			r.Node = "per connection"
		}
	}
	var response struct{ Connections []routeConnection }
	r.SelectedNode, r.SelectedProvider = r.Node, r.Provider
	if err := m.getJSON(ctx, "/connections", &response); err != nil {
		return r, err
	}
	applyObservedRoute(&r, response.Connections, proxies.Proxies)
	return r, nil
}

func applyObservedRoute(r *OuterRoute, connections []routeConnection, proxies map[string]routeProxy) {
	nodes, providers, chains := map[string]bool{}, map[string]bool{}, map[string]bool{}
	mismatch := false
	for _, c := range connections {
		// Restrict evidence to WARP tunnel UDP flows, excluding daemon API TCP calls.
		if !strings.EqualFold(c.Metadata.Network, "udp") {
			continue
		}
		if c.Metadata.Process != "warp-svc" && filepath.Base(c.Metadata.ProcessPath) != "warp-svc" &&
			!(strings.EqualFold(c.Rule, "ProcessName") && c.RulePayload == "warp-svc") {
			continue
		}
		guarded := strings.EqualFold(c.Rule, "ProcessName") && c.RulePayload == "warp-svc"
		targetFound := false
		for _, name := range c.Chains {
			if name == r.Target {
				targetFound = true
			}
			if name == runtimeProxy || name == "WARP-LOCAL" {
				guarded = false
			}
		}
		if !guarded || !targetFound || len(c.Chains) == 0 {
			mismatch = true
		}
		if len(c.Chains) > 0 {
			// Mihomo reports the terminal proxy first, followed by its policy groups.
			nodes[c.Chains[0]] = true
			if p := proxies[c.Chains[0]].Provider; p != "" {
				providers[p] = true
			}
			chains[strings.Join(c.Chains, " <- ")] = true
		}
		for _, p := range c.ProviderChains {
			if p != "" {
				providers[p] = true
			}
		}
	}
	if len(nodes) == 0 && !mismatch {
		return
	}
	r.Node = joinRouteValues(nodes)
	if r.Node == "" {
		r.Node = "---"
	}
	r.Provider = "---"
	if len(providers) > 0 {
		r.Provider = joinRouteValues(providers)
	}
	r.Chains = sortedRouteValues(chains)
	r.Status = "OBSERVED"
	if mismatch {
		r.Status = "MISMATCH"
	}
	if !mismatch && r.Target == "DIRECT" {
		r.Status = "OBSERVED DIRECT"
	}
}

func sortedRouteValues(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func joinRouteValues(values map[string]bool) string {
	return strings.Join(sortedRouteValues(values), " / ")
}
