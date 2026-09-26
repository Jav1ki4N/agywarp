package pages

import (
	"fmt"

	"agywarp/internal/proxymode"
)

type modeSetter interface{ SetProxyMode(proxymode.Mode) }

func (h *Home) applyProxyMode(mode proxymode.Mode) {
	h.proxyMode = mode
	if setter, ok := h.clashManager.(modeSetter); ok {
		setter.SetProxyMode(mode)
	}
	if setter, ok := h.checker.(modeSetter); ok {
		setter.SetProxyMode(mode)
	}
	h.networkCard.ProxyMode = mode.Label()
}
func (h *Home) initProxyMode() {
	path, err := proxymode.SettingsPath()
	h.settingsPath = path
	mode := proxymode.SOCKS5
	if err == nil {
		mode, err = proxymode.Load(path)
	}
	if err != nil {
		h.console.AddLog("WARN", fmt.Sprintf("Proxy preference: %v; using SOCKS5", err))
	}
	if h.tunnelActive {
		if reader, ok := h.clashManager.(interface {
			SessionProxyMode() (proxymode.Mode, error)
		}); ok {
			activeMode, err := reader.SessionProxyMode()
			if err == nil {
				mode = activeMode
			} else {
				h.console.AddLog("WARN", fmt.Sprintf("Cannot read active proxy mode: %v", err))
			}
		}
	}
	h.applyProxyMode(mode)
}
func (h *Home) switchProxyMode() {
	if h.tunnelActive || h.tunnelBusy || h.refreshingNetwork {
		h.console.AddLog("WARN", "Proxy mode is locked while ON, testing, or refreshing; switch when OFF and idle")
		return
	}
	mode := proxymode.HTTP
	if h.proxyMode == proxymode.HTTP {
		mode = proxymode.SOCKS5
	}
	if err := proxymode.Save(h.settingsPath, mode); err != nil {
		h.console.AddLog("ERR", fmt.Sprintf("Cannot save proxy mode: %v", err))
		return
	}
	h.applyProxyMode(mode)
	if mode == proxymode.HTTP {
		h.console.AddLog("WARN", "HTTP CONNECT supports TCP targets; this local proxy does not carry process UDP traffic")
	}
	h.networkCard.ExitIP = "---"
	h.networkCard.ExitCountry = "---"
	h.networkCard.Colo = "---"
	h.networkCard.Latency = "---"
	h.console.AddLog("INFO", "Local WARP proxy mode: "+mode.Label()+"; press t to test this mode (no automatic fallback)")
}
