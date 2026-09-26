package clash

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectOuterRoute(t *testing.T) {
	for _, tc := range []struct {
		name, connections, status, node, provider string
	}{
		{"observed", `[{"metadata":{"network":"udp","processPath":"/usr/bin/warp-svc"},"chains":["US node","Outer"],"providerChains":["Subscription"],"rule":"ProcessName","rulePayload":"warp-svc"}]`, "OBSERVED", "US node", "Subscription"},
		{"unobserved", `[]`, "UNOBSERVED", "US node", "Subscription"},
		{"tcp control is not tunnel evidence", `[{"metadata":{"network":"tcp","process":"warp-svc"},"chains":["US node","Outer"],"rule":"ProcessName","rulePayload":"warp-svc"}]`, "UNOBSERVED", "US node", "Subscription"},
		{"wrong rule", `[{"metadata":{"network":"udp","process":"warp-svc"},"chains":["US node","Outer"],"rule":"Match","rulePayload":""}]`, "MISMATCH", "US node", "Subscription"},
		{"wrong route", `[{"metadata":{"network":"udp","process":"warp-svc"},"chains":["DIRECT"],"rule":"ProcessName","rulePayload":"warp-svc"}]`, "MISMATCH", "DIRECT", "---"},
		{"unrelated process", `[{"metadata":{"network":"udp","process":"browser"},"chains":["Other"]}]`, "UNOBSERVED", "US node", "Subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "clash-verge.yaml"), []byte("proxies: []\nrules:\n  - MATCH,Outer\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("current: airport\nitems:\n  - uid: airport\n    name: My Airport\n"), 0600); err != nil {
				t.Fatal(err)
			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/proxies":
					fmt.Fprint(w, `{"proxies":{"Outer":{"type":"Selector","now":"US node"},"US node":{"type":"VLESS","provider-name":"Subscription"}}}`)
				case "/connections":
					fmt.Fprintf(w, `{"connections":%s}`, tc.connections)
				default:
					http.NotFound(w, r)
				}
			})
			m := &SystemdManager{BaseDir: dir, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, r)
				return recorder.Result(), nil
			})}}
			route, err := m.InspectOuterRoute(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if route.Status != tc.status || route.Node != tc.node || route.Provider != tc.provider || route.Airport != "My Airport" {
				t.Fatalf("route=%+v", route)
			}
		})
	}
}
