package pages

import (
	"agywarp/internal/checker"
	"agywarp/internal/clash"
	"agywarp/internal/proxymode"
	"agywarp/internal/tui/components"
	"path/filepath"
	"testing"
)

func TestProxyModeSwitchPersistenceAndLocks(t *testing.T) {
	h := &Home{settingsPath: filepath.Join(t.TempDir(), "settings.json"), checker: checker.NewChecker(), clashManager: &clash.SystemdManager{}, console: components.NewConsole(), networkCard: components.NewNetworkCard()}
	h.applyProxyMode(proxymode.SOCKS5)
	for _, state := range []string{"active", "busy", "refreshing"} {
		h.tunnelActive = state == "active"
		h.tunnelBusy = state == "busy"
		h.refreshingNetwork = state == "refreshing"
		h.switchProxyMode()
		if h.proxyMode != proxymode.SOCKS5 {
			t.Fatal("switched while " + state)
		}
	}
	h.tunnelActive = false
	h.tunnelBusy = false
	h.refreshingNetwork = false
	h.switchProxyMode()
	if h.proxyMode != proxymode.HTTP || h.networkCard.ProxyMode != "HTTP CONNECT" {
		t.Fatal("HTTP switch failed")
	}
	if mode, err := proxymode.Load(h.settingsPath); err != nil || mode != proxymode.HTTP {
		t.Fatalf("%s %v", mode, err)
	}
	h.settingsPath = ""
	h.switchProxyMode()
	if h.proxyMode != proxymode.HTTP {
		t.Fatal("failed save changed mode")
	}
}
