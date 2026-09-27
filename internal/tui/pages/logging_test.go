package pages

import (
	"strings"
	"testing"
	"time"

	"agywarp/internal/checker"
	"agywarp/internal/clash"
	"agywarp/internal/tui/components"
	"agywarp/internal/warp"
)

func TestUnchangedDisconnectedRefreshIsQuiet(t *testing.T) {
	h := &Home{console: components.NewConsole(), networkCard: components.NewNetworkCard()}
	warpMsg := WarpDetectedMsg{Installed: true, Version: "2026.7", Status: warp.StatusInfo{Status: "DISCONNECTED", Protocol: "MASQUE", ProxyPort: 40000}}
	clashMsg := ClashDetectedMsg{Inspection: clash.Inspection{Installed: true, SocketAvailable: true, MihomoVersion: "v1.19", SummaryStatus: "INACTIVE"}}
	h.Update(warpMsg)
	h.Update(clashMsg)
	count := len(h.console.Logs)
	for i := 0; i < 3; i++ {
		h.Update(warpMsg)
		h.Update(clashMsg)
	}
	if len(h.console.Logs) != count {
		t.Fatalf("unchanged state produced logs: %+v", h.console.Logs)
	}
	warpMsg.Status.Status = "CONNECTED"
	h.Update(warpMsg)
	if len(h.console.Logs) != count+1 {
		t.Fatal("status change was not reported once")
	}
}

func TestAPIProbeFailuresMergeOnlyMatchingCauses(t *testing.T) {
	h := &Home{console: components.NewConsole()}
	results := []checker.APIProbeResult{
		{Endpoint: "https://one.googleapis.com/models", FailureStage: "SOCKS/target connection", Error: "Get one: host unreachable", Latency: 5 * time.Second},
		{Endpoint: "https://two.googleapis.com/models", FailureStage: "SOCKS/target connection", Error: "Get two: host unreachable", Latency: 5005 * time.Millisecond},
		{Endpoint: "https://three.googleapis.com/models", FailureStage: "TLS handshake", Error: "TLS handshake timeout", Latency: 6 * time.Second},
	}
	h.showAPIProbes(results)
	if len(h.console.Logs) != 2 || !strings.Contains(h.console.Logs[0].Message, "2/3") || !strings.Contains(h.console.Logs[1].Message, "TLS handshake") {
		t.Fatalf("logs %+v", h.console.Logs)
	}
	h.console.Clear()
	results[2] = results[0]
	h.showAPIProbes(results)
	if len(h.console.Logs) != 1 || !strings.Contains(h.console.Logs[0].Message, "3/3") {
		t.Fatalf("logs %+v", h.console.Logs)
	}
}
