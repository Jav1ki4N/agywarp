package clash

import (
	"agywarp/internal/proxymode"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const runtimeProxy = "AGYWARP-WARP"

// RuntimeCheck contains the facts needed before changing the live Mihomo config.
type RuntimeCheck struct {
	BasePath          string
	BaseHash          string
	SocketPath        string
	Version           string
	TUN               bool
	ProcessMatching   bool
	Active            bool
	WarpConnectedByUs bool
	Rules             []string
}

type runtimeSession struct {
	ProxyMode         proxymode.Mode `json:"proxy_mode,omitempty"`
	BasePath          string         `json:"base_path"`
	BaseHash          string         `json:"base_hash"`
	Rules             []string       `json:"rules"`
	WarpConnectedByUs bool           `json:"warp_connected_by_us"`
	Route             routeSnapshot  `json:"route,omitempty"`
}

func (m *SystemdManager) basePath() string { return filepath.Join(m.BaseDir, "clash-verge.yaml") }
func (m *SystemdManager) sessionPath() string {
	return filepath.Join(m.BaseDir, ".agywarp", "session.json")
}

func (m *SystemdManager) controller(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	client := m.HTTPClient
	if client == nil {
		timeout := 5 * time.Second
		if method == http.MethodPut {
			timeout = 20 * time.Second
		}
		client = &http.Client{Transport: &http.Transport{DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(c, "unix", m.SocketPath)
		}}, Timeout: timeout}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

func (m *SystemdManager) getJSON(ctx context.Context, path string, dest any) error {
	resp, err := m.controller(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func (m *SystemdManager) loadPayload(ctx context.Context, config []byte) error {
	body, err := json.Marshal(map[string]string{"path": "", "payload": string(config)})
	if err != nil {
		return err
	}
	resp, err := m.controller(ctx, http.MethodPut, "/configs?force=true", body)
	if err != nil {
		return fmt.Errorf("Mihomo reload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("Mihomo reload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func configNode(base []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(base, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("base config must be a YAML mapping")
	}
	return &doc, doc.Content[0], nil
}

func field(root *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// BuildRuntime creates a complete config in memory. The source bytes are never modified.
func BuildRuntime(base []byte, rules []string, port int) ([]byte, error) {
	return BuildRuntimeWithMode(base, rules, port, proxymode.SOCKS5)
}

func BuildRuntimeWithMode(base []byte, rules []string, port int, mode proxymode.Mode) ([]byte, error) {
	if !mode.Valid() {
		return nil, fmt.Errorf("invalid proxy mode %q", mode)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid WARP proxy port %d", port)
	}
	doc, root, err := configNode(base)
	if err != nil {
		return nil, err
	}
	proxies := field(root, "proxies")
	if proxies == nil {
		proxies = &yaml.Node{Kind: yaml.SequenceNode}
		root.Content = append(root.Content, scalar("proxies"), proxies)
	}
	if proxies.Kind != yaml.SequenceNode {
		return nil, errors.New("proxies must be a list")
	}
	for _, p := range proxies.Content {
		if name := field(p, "name"); name != nil && name.Value == runtimeProxy {
			return nil, fmt.Errorf("proxy name %s already exists", runtimeProxy)
		}
	}
	proxy := &yaml.Node{Kind: yaml.MappingNode}
	for _, kv := range [][2]string{{"name", runtimeProxy}, {"type", string(mode)}, {"server", "127.0.0.1"}, {"port", fmt.Sprint(port)}} {
		v := scalar(kv[1])
		if kv[0] == "port" {
			v.Tag = "!!int"
		}
		proxy.Content = append(proxy.Content, scalar(kv[0]), v)
	}
	proxies.Content = append(proxies.Content, proxy)
	ruleList := field(root, "rules")
	if ruleList == nil || ruleList.Kind != yaml.SequenceNode {
		return nil, errors.New("rules must be a list")
	}
	var insert []*yaml.Node
	outer := "DIRECT"
	for _, r := range ruleList.Content {
		if strings.HasPrefix(r.Value, "PROCESS-NAME,warp-svc,") {
			return nil, errors.New("base config contains persistent warp-svc rule")
		}
		if strings.HasPrefix(r.Value, "MATCH,") {
			parts := strings.Split(r.Value, ",")
			if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
				outer = parts[1]
			}
		}
	}
	if outer == runtimeProxy || outer == "WARP-LOCAL" || outer == "REJECT" || outer == "REJECT-DROP" {
		return nil, fmt.Errorf("default route %q cannot carry WARP", outer)
	}
	insert = append(insert, scalar("PROCESS-NAME,warp-svc,"+outer))
	seen := map[string]bool{}
	for _, r := range rules {
		parts := strings.Split(r, ",")
		if len(parts) != 3 || (parts[0] != "PROCESS-NAME" && parts[0] != "PROCESS-PATH") || strings.TrimSpace(parts[1]) == "" || parts[2] != runtimeProxy {
			return nil, fmt.Errorf("invalid process rule %q", r)
		}
		if strings.ContainsAny(parts[1], "\r\n") {
			return nil, fmt.Errorf("invalid process rule %q", r)
		}
		if !seen[r] {
			insert = append(insert, scalar(r))
			seen[r] = true
		}
	}
	ruleList.Content = append(insert, ruleList.Content...)
	if mode := field(root, "mode"); mode != nil {
		mode.Value = "rule"
	} else {
		root.Content = append(root.Content, scalar("mode"), scalar("rule"))
	}
	return yaml.Marshal(doc)
}

func (m *SystemdManager) liveRules(ctx context.Context) ([]string, error) {
	var response struct {
		Rules []struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
		} `json:"rules"`
	}
	if err := m.getJSON(ctx, "/rules", &response); err != nil {
		return nil, err
	}
	var found []string
	for _, r := range response.Rules {
		if r.Proxy == runtimeProxy {
			typeName := strings.ToUpper(strings.ReplaceAll(r.Type, "-", ""))
			switch typeName {
			case "PROCESSNAME":
				typeName = "PROCESS-NAME"
			case "PROCESSPATH":
				typeName = "PROCESS-PATH"
			}
			found = append(found, typeName+","+r.Payload+","+r.Proxy)
		}
	}
	return found, nil
}

// liveRuntimeArtifacts also finds a bootstrap left behind before app rules or
// a session could be written.
func (m *SystemdManager) liveRuntimeArtifacts(ctx context.Context) ([]string, error) {
	var rules struct {
		Rules []struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
		} `json:"rules"`
	}
	if err := m.getJSON(ctx, "/rules", &rules); err != nil {
		return nil, err
	}
	var found []string
	for _, rule := range rules.Rules {
		if rule.Proxy == runtimeProxy || rule.Proxy == "WARP-LOCAL" || strings.EqualFold(strings.ReplaceAll(rule.Type, "-", ""), "ProcessName") && rule.Payload == "warp-svc" {
			found = append(found, rule.Type+","+rule.Payload+","+rule.Proxy)
		}
	}
	var proxies struct {
		Proxies map[string]json.RawMessage `json:"proxies"`
	}
	if err := m.getJSON(ctx, "/proxies", &proxies); err != nil {
		return nil, err
	}
	for _, name := range []string{runtimeProxy, "WARP-LOCAL"} {
		if _, ok := proxies.Proxies[name]; ok {
			found = append(found, "proxy:"+name)
		}
	}
	return found, nil
}

func (m *SystemdManager) Preflight(ctx context.Context) (RuntimeCheck, error) {
	return m.preflight(ctx, false)
}

func (m *SystemdManager) preflight(ctx context.Context, allowBootstrap bool) (RuntimeCheck, error) {
	check := RuntimeCheck{BasePath: m.basePath(), SocketPath: m.SocketPath}
	base, err := os.ReadFile(check.BasePath)
	if err != nil {
		return check, fmt.Errorf("read base config: %w", err)
	}
	h := sha256.Sum256(base)
	check.BaseHash = hex.EncodeToString(h[:])
	_, root, err := configNode(base)
	if err != nil {
		return check, fmt.Errorf("parse base config: %w", err)
	}
	if field(root, "rules") == nil {
		return check, errors.New("base config has no rules")
	}
	if proxies := field(root, "proxies"); proxies != nil && proxies.Kind == yaml.SequenceNode {
		for _, proxy := range proxies.Content {
			if name := field(proxy, "name"); name != nil && (name.Value == runtimeProxy || name.Value == "WARP-LOCAL") {
				return check, fmt.Errorf("base config contains persistent WARP proxy %q (at %s:%d)", name.Value, check.BasePath, name.Line)
			}
		}
	}
	for _, r := range field(root, "rules").Content {
		if strings.HasPrefix(r.Value, "PROCESS-NAME,warp-svc,") || strings.HasSuffix(r.Value, ","+runtimeProxy) || strings.HasSuffix(r.Value, ",WARP-LOCAL") {
			return check, fmt.Errorf("base config contains a persistent WARP routing rule: %s (at %s:%d)", r.Value, check.BasePath, r.Line)
		}
	}
	if _, err := BuildRuntime(base, nil, 40000); err != nil {
		return check, err
	}
	var ver struct {
		Version string `json:"version"`
	}
	if err := m.getJSON(ctx, "/version", &ver); err != nil {
		return check, fmt.Errorf("Mihomo controller unavailable: %w", err)
	}
	check.Version = ver.Version
	var cfg map[string]json.RawMessage
	if err := m.getJSON(ctx, "/configs", &cfg); err != nil {
		return check, fmt.Errorf("Mihomo config inspection: %w", err)
	}
	var tun struct {
		Enable bool `json:"enable"`
	}
	_ = json.Unmarshal(cfg["tun"], &tun)
	check.TUN = tun.Enable
	var processMode string
	_ = json.Unmarshal(cfg["find-process-mode"], &processMode)
	check.ProcessMatching = processMode != "off"
	if !check.TUN {
		return check, errors.New("Mihomo TUN is disabled; enable TUN in Clash Verge and retry")
	}
	if !check.ProcessMatching {
		return check, errors.New("Mihomo process matching is disabled")
	}
	check.Rules, err = m.liveRules(ctx)
	if err != nil {
		return check, err
	}
	artifacts, err := m.liveRuntimeArtifacts(ctx)
	if err != nil {
		return check, err
	}
	sessionData, sessionErr := os.ReadFile(m.sessionPath())
	check.Active = len(check.Rules) > 0
	if len(artifacts) > 0 && sessionErr != nil {
		validBootstrap := allowBootstrap && !check.Active && len(artifacts) == 2
		if validBootstrap {
			hasProxy, hasGuard := false, false
			for _, artifact := range artifacts {
				hasProxy = hasProxy || artifact == "proxy:"+runtimeProxy
				hasGuard = hasGuard || strings.Contains(artifact, ",warp-svc,")
			}
			validBootstrap = hasProxy && hasGuard
		}
		if !validBootstrap {
			return check, fmt.Errorf("orphaned agywarp runtime artifacts %v; recover live Mihomo config before continuing", artifacts)
		}
	}
	if !check.Active && sessionErr == nil {
		return check, errors.New("stale agywarp session; run 'agywarp recover' after confirming Mihomo has no agywarp rules")
	}
	if check.Active {
		hasProxy, hasGuard := false, false
		for _, artifact := range artifacts {
			hasProxy = hasProxy || artifact == "proxy:"+runtimeProxy
			hasGuard = hasGuard || strings.Contains(artifact, ",warp-svc,")
		}
		if !hasProxy || !hasGuard {
			return check, fmt.Errorf("incomplete agywarp runtime: WARP proxy or warp-svc guard missing")
		}
		var session runtimeSession
		if err := json.Unmarshal(sessionData, &session); err != nil {
			return check, fmt.Errorf("read session: %w", err)
		}
		if session.BasePath != check.BasePath {
			return check, errors.New("runtime session belongs to another base config")
		}
		liveSet := make(map[string]bool, len(check.Rules))
		for _, rule := range check.Rules {
			liveSet[rule] = true
		}
		for _, rule := range session.Rules {
			if !liveSet[rule] {
				return check, fmt.Errorf("runtime rule missing: %s", rule)
			}
			delete(liveSet, rule)
		}
		if len(liveSet) > 0 {
			return check, fmt.Errorf("unexpected runtime rules: %v", liveSet)
		}
		check.WarpConnectedByUs = session.WarpConnectedByUs
	}
	return check, nil
}

func (m *SystemdManager) StartRuntime(ctx context.Context, rules []string, port int, connectedByUs bool) (int, error) {
	check, err := m.preflight(ctx, true)
	if err != nil {
		return 0, err
	}
	if check.Active {
		return 0, errors.New("runtime already active")
	}
	route, err := m.currentRoute(ctx)
	if err != nil {
		return 0, fmt.Errorf("inspect active airport and node: %w", err)
	}
	base, err := os.ReadFile(check.BasePath)
	if err != nil {
		return 0, err
	}
	runtime, err := BuildRuntimeWithMode(base, rules, port, m.proxyMode.Get())
	if err != nil {
		return 0, err
	}
	if err := m.loadPayload(ctx, runtime); err != nil {
		return 0, err
	}
	expected := map[string]bool{}
	for _, r := range rules {
		expected[r] = true
	}
	live, err := m.liveRules(ctx)
	if err == nil {
		for _, r := range live {
			delete(expected, r)
		}
	}
	if err != nil || len(expected) > 0 {
		rollbackErr := m.loadPayload(ctx, base)
		return 0, fmt.Errorf("verify runtime rules: missing %v, read error %v; rollback: %v", expected, err, rollbackErr)
	}
	session := runtimeSession{ProxyMode: m.proxyMode.Get(), BasePath: check.BasePath, BaseHash: check.BaseHash, Rules: rules, WarpConnectedByUs: connectedByUs, Route: route}
	data, _ := json.MarshalIndent(session, "", "  ")
	if err := writeSession(m.sessionPath(), data); err != nil {
		rollbackErr := m.loadPayload(ctx, base)
		return 0, fmt.Errorf("save session: %w; rollback: %v", err, rollbackErr)
	}
	return len(rules), nil
}

func writeSession(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "session-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		os.Remove(path)
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// BootstrapRuntime installs only the WARP proxy and warp-svc guard. No app is
// routed through WARP until StartRuntime succeeds.
func (m *SystemdManager) BootstrapRuntime(ctx context.Context, port int) error {
	check, err := m.Preflight(ctx)
	if err != nil {
		return err
	}
	if check.Active {
		return errors.New("runtime already active")
	}
	base, err := os.ReadFile(check.BasePath)
	if err != nil {
		return err
	}
	bootstrap, err := BuildRuntimeWithMode(base, nil, port, m.proxyMode.Get())
	if err != nil {
		return err
	}
	return m.loadPayload(ctx, bootstrap)
}

// AbortBootstrap restores the untouched base if startup fails before app rules
// are committed. A failed restore is returned to the caller for safe handling.
func (m *SystemdManager) AbortBootstrap(ctx context.Context) error {
	base, err := os.ReadFile(m.basePath())
	if err != nil {
		return err
	}
	if err := m.restoreBase(ctx, base); err != nil {
		return err
	}
	live, err := m.liveRuntimeArtifacts(ctx)
	if err != nil {
		return err
	}
	if len(live) > 0 {
		return fmt.Errorf("runtime rules remain after bootstrap restore: %v", live)
	}
	return nil
}

// A timed-out PUT has an unknown outcome. Verify cleanup before reporting failure;
// never disconnect WARP merely because the controller stopped answering.
func (m *SystemdManager) restoreBase(ctx context.Context, base []byte) error {
	err := m.loadPayload(ctx, base)
	if err == nil {
		return nil
	}
	var networkError net.Error
	if !(errors.As(err, &networkError) && networkError.Timeout()) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if ctx.Err() != nil {
		return err
	}
	live, inspectErr := m.liveRuntimeArtifacts(ctx)
	if inspectErr == nil && len(live) == 0 {
		return nil
	}
	if inspectErr != nil {
		return fmt.Errorf("%w; cannot verify cleanup: %v", err, inspectErr)
	}
	return fmt.Errorf("%w; runtime artifacts still present: %v", err, live)
}

func (m *SystemdManager) StopRuntime(ctx context.Context) (bool, error) {
	data, err := os.ReadFile(m.sessionPath())
	if err != nil {
		return false, fmt.Errorf("read runtime session: %w", err)
	}
	var session runtimeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return false, err
	}
	if session.BasePath != m.basePath() {
		return false, errors.New("runtime session belongs to another base config")
	}
	base, err := os.ReadFile(session.BasePath)
	if err != nil {
		return false, err
	}
	h := sha256.Sum256(base)
	if hex.EncodeToString(h[:]) != session.BaseHash {
		// Clash Verge may have switched profiles while agywarp was ON. Restore
		// the new generated base only if it has no persistent WARP routing.
		_, root, parseErr := configNode(base)
		if parseErr != nil {
			return false, fmt.Errorf("changed base config is invalid: %w", parseErr)
		}
		rules := field(root, "rules")
		if rules == nil || rules.Kind != yaml.SequenceNode {
			return false, errors.New("changed base config has no rules list")
		}
		for _, rule := range rules.Content {
			if strings.HasPrefix(rule.Value, "PROCESS-NAME,warp-svc,") || strings.HasSuffix(rule.Value, ",WARP-LOCAL") || strings.HasSuffix(rule.Value, ","+runtimeProxy) {
				return false, fmt.Errorf("changed base config contains persistent WARP rule %q; clean the profile extension first", rule.Value)
			}
		}
	}
	if err := m.cleanLegacyWARP(); err != nil {
		return false, fmt.Errorf("clean legacy profile WARP entries: %w", err)
	}
	if err := m.restoreBase(ctx, base); err != nil {
		return false, err
	}
	live, err := m.liveRuntimeArtifacts(ctx)
	if err != nil {
		return false, err
	}
	if len(live) > 0 {
		return false, fmt.Errorf("runtime rules remain after restore: %v", live)
	}
	if err := os.Remove(m.sessionPath()); err != nil {
		return false, err
	}
	return session.WarpConnectedByUs, nil
}

// RecoverRuntime is an explicit repair for orphaned rules or a stale session.
// It restores only Mihomo; WARP remains in its current state.
func (m *SystemdManager) RecoverRuntime(ctx context.Context) error {
	base, err := os.ReadFile(m.basePath())
	if err != nil {
		return err
	}
	_, root, err := configNode(base)
	if err != nil {
		return err
	}
	if rules := field(root, "rules"); rules == nil || rules.Kind != yaml.SequenceNode {
		return errors.New("base config has no valid rules list")
	}
	for _, r := range field(root, "rules").Content {
		if strings.HasSuffix(r.Value, ","+runtimeProxy) {
			return errors.New("base config contains runtime rules")
		}
	}
	data, sessionErr := os.ReadFile(m.sessionPath())
	if sessionErr != nil && !os.IsNotExist(sessionErr) {
		return sessionErr
	}
	if sessionErr == nil {
		var session runtimeSession
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}
		h := sha256.Sum256(base)
		if session.BasePath != m.basePath() || session.BaseHash != hex.EncodeToString(h[:]) {
			return errors.New("base config changed since runtime activation")
		}
	}
	live, err := m.liveRuntimeArtifacts(ctx)
	if err != nil {
		return err
	}
	if len(live) == 0 && sessionErr != nil {
		return errors.New("no orphaned runtime or stale session found")
	}
	if len(live) > 0 && sessionErr == nil {
		return errors.New("runtime session is active; stop from Network Card")
	}
	if err := m.restoreBase(ctx, base); err != nil {
		return err
	}
	live, err = m.liveRuntimeArtifacts(ctx)
	if err != nil {
		return err
	}
	if len(live) > 0 {
		return fmt.Errorf("runtime rules remain after recovery: %v", live)
	}
	if sessionErr == nil {
		return os.Remove(m.sessionPath())
	}
	return nil
}

// SessionProxyMode reads the protocol used when the active runtime was created.
func (m *SystemdManager) SessionProxyMode() (proxymode.Mode, error) {
	data, err := os.ReadFile(m.sessionPath())
	if err != nil {
		return proxymode.SOCKS5, err
	}
	var session runtimeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return proxymode.SOCKS5, err
	}
	if session.ProxyMode == "" {
		return proxymode.SOCKS5, nil
	}
	if !session.ProxyMode.Valid() {
		return proxymode.SOCKS5, fmt.Errorf("invalid session proxy mode")
	}
	return session.ProxyMode, nil
}
