package clash

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agywarp/internal/proxymode"
	"gopkg.in/yaml.v3"
)

func TestBuildRuntimeDoesNotAlterBaseAndOrdersRules(t *testing.T) {
	base := []byte("mode: global\nproxies: []\nrules:\n  - MATCH,MyGroup\n")
	original := append([]byte(nil), base...)
	result, err := BuildRuntime(base, []string{"PROCESS-PATH,/usr/bin/example,AGYWARP-WARP"}, 40000)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base, original) {
		t.Fatal("source config was changed")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(result, &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	if got := field(root, "mode").Value; got != "rule" {
		t.Fatalf("mode=%s", got)
	}
	proxies := field(root, "proxies")
	if len(proxies.Content) != 1 || field(proxies.Content[0], "name").Value != runtimeProxy {
		t.Fatal("runtime proxy missing")
	}
	rules := field(root, "rules").Content
	if len(rules) != 3 || rules[0].Value != "PROCESS-NAME,warp-svc,MyGroup" || rules[1].Value != "PROCESS-PATH,/usr/bin/example,AGYWARP-WARP" || rules[2].Value != "MATCH,MyGroup" {
		t.Fatalf("wrong rule order: %s", strings.TrimSpace(string(result)))
	}
}

func TestRuntimeLifecycleUsesPayloadAndPreservesBase(t *testing.T) {
	dir := t.TempDir()
	base := []byte("mode: rule\ntun: {enable: true}\nproxies: []\nrules: [MATCH,DIRECT]\n")
	basePath := filepath.Join(dir, "clash-verge.yaml")
	if err := os.WriteFile(basePath, base, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("current: test-profile\nitems: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := base
	selectorNow := "NodeA"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/configs" {
			var req struct {
				Payload string `json:"payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			loaded = []byte(req.Payload)
			w.WriteHeader(204)
			return
		}
		switch r.URL.Path {
		case "/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "test"})
		case "/configs":
			json.NewEncoder(w).Encode(map[string]any{"tun": map[string]bool{"enable": true}, "find-process-mode": "strict"})
		case "/proxies":
			proxies := map[string]any{"ProxyGroup": map[string]any{"type": "Selector", "now": selectorNow}}
			var doc yaml.Node
			if err := yaml.Unmarshal(loaded, &doc); err == nil {
				if list := field(doc.Content[0], "proxies"); list != nil {
					for _, node := range list.Content {
						if name := field(node, "name"); name != nil {
							proxies[name.Value] = map[string]any{"type": "Socks5"}
						}
					}
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"proxies": proxies})
		case "/rules":
			var doc yaml.Node
			_ = yaml.Unmarshal(loaded, &doc)
			var rules []map[string]string
			for _, n := range field(doc.Content[0], "rules").Content {
				parts := strings.Split(n.Value, ",")
				if len(parts) == 3 && (parts[2] == runtimeProxy || parts[1] == "warp-svc") {
					rules = append(rules, map[string]string{"type": strings.ReplaceAll(parts[0], "-", ""), "payload": parts[1], "proxy": parts[2]})
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rules": rules})
		default:
			http.NotFound(w, r)
		}
	})
	m := &SystemdManager{BaseDir: dir, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		return recorder.Result(), nil
	})}}
	m.SetProxyMode(proxymode.HTTP)
	ctx := context.Background()
	if err := m.BootstrapRuntime(ctx, 40000); err != nil {
		t.Fatal(err)
	}
	var bootstrapDoc yaml.Node
	if err := yaml.Unmarshal(loaded, &bootstrapDoc); err != nil {
		t.Fatal(err)
	}
	if got := field(field(bootstrapDoc.Content[0], "proxies").Content[0], "type").Value; got != "http" {
		t.Fatalf("bootstrap proxy type %s", got)
	}
	rule := "PROCESS-NAME,example,AGYWARP-WARP"
	if _, err := m.StartRuntime(ctx, []string{rule}, 40000, true); err != nil {
		t.Fatal(err)
	}
	if mode, err := m.SessionProxyMode(); err != nil || mode != proxymode.HTTP {
		t.Fatalf("session mode %s %v", mode, err)
	}
	check, err := m.Preflight(ctx)
	if err != nil || !check.Active || !check.WarpConnectedByUs {
		t.Fatalf("runtime state: %+v, %v", check, err)
	}
	if change, err := m.RuntimeRouteChanged(ctx); err != nil || change != "" {
		t.Fatalf("unchanged route: %q %v", change, err)
	}
	selectorNow = "NodeB"
	if change, err := m.RuntimeRouteChanged(ctx); err != nil || !strings.Contains(change, "node selection changed") {
		t.Fatalf("changed node: %q %v", change, err)
	}
	selectorNow = "NodeA"
	if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("current: other-profile\nitems: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if change, err := m.RuntimeRouteChanged(ctx); err != nil || change != "airport profile changed" {
		t.Fatalf("changed airport: %q %v", change, err)
	}
	newBase := []byte("mode: rule\ntun: {enable: true}\nproxies: []\nrules: [MATCH,NewGroup]\n")
	if err := os.WriteFile(basePath, newBase, 0600); err != nil {
		t.Fatal(err)
	}
	if change, err := m.RuntimeRouteChanged(ctx); err != nil || change != "generated Mihomo config changed" {
		t.Fatalf("changed generated config: %q %v", change, err)
	}
	owned, err := m.StopRuntime(ctx)
	if err != nil || !owned {
		t.Fatalf("stop: owned=%v err=%v", owned, err)
	}
	after, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, newBase) {
		t.Fatal("base config was modified")
	}
	if !bytes.Equal(loaded, newBase) {
		t.Fatal("live config was not restored")
	}
	if err := os.WriteFile(basePath, base, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.BootstrapRuntime(ctx, 40000); err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartRuntime(ctx, []string{rule}, 40000, true); err != nil {
		t.Fatal(err)
	}
	loaded = base // Simulate Clash Verge replacing the live config behind agywarp.
	if _, err := m.Preflight(ctx); err == nil || !strings.Contains(err.Error(), "stale agywarp session") {
		t.Fatalf("expected stale session, got %v", err)
	}
	if err := m.RecoverRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.sessionPath()); !os.IsNotExist(err) {
		t.Fatalf("session still exists after recovery: %v", err)
	}
	if _, err := m.Preflight(ctx); err != nil {
		t.Fatalf("preflight after recovery: %v", err)
	}
	bootstrap, err := BuildRuntime(base, nil, 40000)
	if err != nil {
		t.Fatal(err)
	}
	loaded = bootstrap // Simulate a crash after bootstrap but before app rules/session.
	if _, err := m.Preflight(ctx); err == nil || !strings.Contains(err.Error(), "orphaned agywarp runtime artifacts") {
		t.Fatalf("expected orphaned bootstrap, got %v", err)
	}
	if err := m.RecoverRuntime(ctx); err != nil {
		t.Fatalf("recover orphaned bootstrap: %v", err)
	}
	if !bytes.Equal(loaded, base) {
		t.Fatal("orphaned bootstrap was not removed")
	}
}

func TestCleanLegacyWARPRemovesOnlyOwnedEntries(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profilesDir, 0700); err != nil {
		t.Fatal(err)
	}
	index := "current: airport\nitems:\n  - uid: airport\n    option:\n      rules: legacy-rules\n      proxies: legacy-proxies\n"
	if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	rules := "prepend:\n  - PROCESS-NAME,warp-svc,Group\n  - PROCESS-NAME,agy,WARP-LOCAL\n  - DOMAIN,example.com,DIRECT\nappend: []\n"
	proxies := "prepend:\n  - name: WARP-LOCAL\n    type: socks5\n    server: 127.0.0.1\n    port: 40000\n  - name: MyProxy\n    type: socks5\n    server: 127.0.0.1\n    port: 10000\nappend: []\n"
	for name, content := range map[string]string{"legacy-rules.yaml": rules, "legacy-proxies.yaml": proxies} {
		if err := os.WriteFile(filepath.Join(profilesDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := &SystemdManager{BaseDir: dir}
	if err := m.cleanLegacyWARP(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, keep, remove string }{{"legacy-rules.yaml", "example.com,DIRECT", "warp-svc"}, {"legacy-proxies.yaml", "MyProxy", "WARP-LOCAL"}} {
		data, err := os.ReadFile(filepath.Join(profilesDir, tc.name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte(tc.keep)) || bytes.Contains(data, []byte(tc.remove)) {
			t.Fatalf("incorrect cleanup of %s: %s", tc.name, data)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBuildRuntimeRejectsUnsafeRules(t *testing.T) {
	base := []byte("proxies: []\nrules: [MATCH,DIRECT]\n")
	for _, rule := range []string{"DOMAIN,example.com,AGYWARP-WARP", "PROCESS-NAME,foo,DIRECT", "PROCESS-NAME,foo,AGYWARP-WARP\nMATCH,DIRECT"} {
		if _, err := BuildRuntime(base, []string{rule}, 40000); err == nil {
			t.Fatalf("accepted %q", rule)
		}
	}
	if _, err := BuildRuntime([]byte("proxies:\n  - name: AGYWARP-WARP\nrules: [MATCH,DIRECT]\n"), nil, 40000); err == nil {
		t.Fatal("accepted reserved proxy collision")
	}
}

func TestPreflightReportsPersistentRule(t *testing.T) {
	dir := t.TempDir()
	base := []byte("proxies: []\nrules:\n  - PROCESS-NAME,agy,WARP-LOCAL\n  - MATCH,DIRECT\n")
	if err := os.WriteFile(filepath.Join(dir, "clash-verge.yaml"), base, 0600); err != nil {
		t.Fatal(err)
	}
	m := &SystemdManager{BaseDir: dir}
	_, err := m.Preflight(context.Background())
	if err == nil || !strings.Contains(err.Error(), "PROCESS-NAME,agy,WARP-LOCAL") || !strings.Contains(err.Error(), ":3") {
		t.Fatalf("expected precise rule location, got %v", err)
	}
}

func TestBuildRuntimeHTTPMode(t *testing.T) {
	base := []byte("proxies: []\nrules: [MATCH,DIRECT]\n")
	result, err := BuildRuntimeWithMode(base, nil, 41000, proxymode.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(result, &doc); err != nil {
		t.Fatal(err)
	}
	proxy := field(doc.Content[0], "proxies").Content[0]
	if field(proxy, "type").Value != "http" || field(proxy, "port").Value != "41000" {
		t.Fatalf("wrong proxy: %s", result)
	}
}
