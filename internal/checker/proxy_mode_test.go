package checker

import (
	"agywarp/internal/proxymode"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Both HTTPS paths must negotiate CONNECT with the explicit proxy. A denied
// tunnel must not silently fall back to a direct target connection.
func TestHTTPModeUsesCONNECTForTraceAndAPI(t *testing.T) {
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "example.invalid:443" {
			t.Errorf("unexpected proxy request %s %s", r.Method, r.Host)
		}
		calls.Add(1)
		http.Error(w, "tunnel denied", http.StatusBadGateway)
	}))
	defer proxy.Close()
	c := &HTTPChecker{TraceURL: "https://example.invalid/trace", ProbeURLs: []string{"https://example.invalid/models"}}
	c.SetProxyMode(proxymode.HTTP)
	addr := strings.TrimPrefix(proxy.URL, "http://")
	if _, err := c.VerifyExit(context.Background(), addr); err == nil || !strings.Contains(err.Error(), "rejected target example.invalid:443: 502") {
		t.Fatalf("trace CONNECT error: %v", err)
	}
	results := c.ProbeAPIs(context.Background(), addr)
	if len(results) != 1 || results[0].Error == "" || results[0].FailureStage != "HTTP CONNECT/target connection" {
		t.Fatalf("%+v", results)
	}
	if calls.Load() != 2 {
		t.Fatalf("CONNECT count = %d", calls.Load())
	}
}
