package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNetworkCardRender(t *testing.T) {
	card := NewNetworkCard()
	card.Width = 40
	card.Height = 8

	// Default unlaunched state
	rendered := card.Render()
	if !strings.Contains(rendered, "WARP Tunnel") {
		t.Errorf("expected header to contain 'WARP Tunnel', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "DISCONNECTED") {
		t.Errorf("expected WARP Tunnel to show 'DISCONNECTED', got:\n%s", rendered)
	}

	// Resolved state
	card.WarpStatus = "CONNECTED"
	card.Protocol = "MASQUE"
	card.ExitCountry = "US"
	card.ExitIP = "104.28.195.192"
	card.Colo = "LAX"
	card.Latency = "35ms"
	card.MihomoRule = "SYNCED (6 rules)"

	renderedDone := card.Render()
	if !strings.Contains(renderedDone, "CONNECTED (MASQUE)") {
		t.Errorf("expected WARP status to show 'CONNECTED (MASQUE)', got:\n%s", renderedDone)
	}
	if !strings.Contains(renderedDone, "104.28.195.192 · LAX") {
		t.Errorf("expected Exit IP to show '104.28.195.192 · LAX', got:\n%s", renderedDone)
	}
	if !strings.Contains(renderedDone, "35ms") {
		t.Errorf("expected Latency to show '35ms', got:\n%s", renderedDone)
	}
	if !strings.Contains(renderedDone, "SYNCED (6 rules)") {
		t.Errorf("expected Mihomo rule to show 'SYNCED (6 rules)', got:\n%s", renderedDone)
	}
}

func TestNetworkCardOuterRoute(t *testing.T) {
	card := NewNetworkCard()
	card.Width, card.Height = 48, 13
	card.OuterNode = "US node"
	card.OuterProvider = "Subscription"
	card.OuterAirport = "My Airport"
	card.OuterStatus = "MISMATCH"
	rendered := card.Render()
	for _, value := range []string{"WARP outer route", "US node", "Subscription", "My Airport", "MISMATCH"} {
		if !strings.Contains(rendered, value) {
			t.Fatalf("missing %q in card: %s", value, rendered)
		}
	}
}

func TestNetworkCardFieldSpinnerNoTitleSpinner(t *testing.T) {
	card := NewNetworkCard()
	card.Width = 40
	card.Height = 8

	card.Refreshing = true
	card.SpinnerView = "/"

	rendered := card.Render()
	lines := strings.Split(rendered, "\n")

	// 1. Verify title line does NOT contain spinner
	if strings.Contains(lines[0], "/") {
		t.Errorf("expected title line NOT to contain spinner '/', got: %q", lines[0])
	}
	if strings.Contains(lines[0], "Refreshing...") {
		t.Errorf("expected title line NOT to contain 'Refreshing...', got: %q", lines[0])
	}

	// 2. Verify metric fields DO contain spinner
	if !strings.Contains(rendered, "resolving...") || !strings.Contains(rendered, "/") {
		t.Errorf("expected field to show resolving with '/', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "measuring...") {
		t.Errorf("expected field to show measuring with '/', got:\n%s", rendered)
	}
}

func TestNetworkCardKeypressRefresh(t *testing.T) {
	card := NewNetworkCard()
	cmd := card.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatalf("expected 'r' keypress to return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(RefreshNetworkMsg); !ok {
		t.Fatalf("expected cmd to emit RefreshNetworkMsg, got %T", msg)
	}
}
