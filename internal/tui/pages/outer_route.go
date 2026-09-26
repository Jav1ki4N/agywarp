package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agywarp/internal/clash"
)

func inspectOuterRoute(ctx context.Context, manager clash.Manager) *clash.OuterRoute {
	inspector, ok := manager.(clash.OuterRouteInspector)
	if !ok {
		return &clash.OuterRoute{Node: "---", Provider: "---", Airport: "---", Status: "UNAVAILABLE"}
	}
	route, err := inspector.InspectOuterRoute(ctx)
	if err != nil {
		route.Status = "UNAVAILABLE"
		route.Detail = err.Error()
	}
	return &route
}

// Sample while bootstrap is still live, before restoring the base.
func observeOuterRoute(ctx context.Context, manager clash.Manager) *clash.OuterRoute {
	timeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var route *clash.OuterRoute
	for {
		route = inspectOuterRoute(timeout, manager)
		if route.Status != "UNOBSERVED" {
			return route
		}
		select {
		case <-timeout.Done():
			return route
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (h *Home) showOuterRoute(route *clash.OuterRoute, log bool) {
	if route == nil {
		return
	}
	h.networkCard.OuterNode = route.Node
	h.networkCard.OuterProvider = route.Provider
	h.networkCard.OuterAirport = route.Airport
	h.networkCard.OuterStatus = route.Status
	if !h.tunnelActive && !log && route.Status != "UNAVAILABLE" {
		h.networkCard.OuterNode = route.SelectedNode
		h.networkCard.OuterProvider = route.SelectedProvider
		h.networkCard.OuterStatus = "OFF (selection)"
	}
	if h.networkCard.OuterNode == "" {
		h.networkCard.OuterNode = "---"
	}
	if h.networkCard.OuterProvider == "" {
		h.networkCard.OuterProvider = "---"
	}
	if h.networkCard.OuterAirport == "" {
		h.networkCard.OuterAirport = "---"
	}
	if !log {
		return
	}
	if strings.HasPrefix(route.Status, "OBSERVED") {
		h.console.AddLog("OK", fmt.Sprintf("WARP UDP route observed: %s; airport: %s; provider: %s; chains: %s", route.Node, route.Airport, route.Provider, strings.Join(route.Chains, "; ")))
	} else {
		h.console.AddLog("WARN", fmt.Sprintf("WARP outer path %s: connectivity alone does not confirm the selected node", route.Status))
		if route.Detail != "" {
			h.console.AddLog("WARN", route.Detail)
		}
	}
}
