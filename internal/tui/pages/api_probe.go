package pages

import (
	"fmt"
	"strings"

	"agywarp/internal/checker"
)

func (h *Home) showAPIProbes(results []checker.APIProbeResult) {
	for _, r := range results {
		if r.Error != "" {
			response := ""
			if r.StatusCode != 0 {
				response = fmt.Sprintf("HTTP %d received; ", r.StatusCode)
			}
			h.console.AddLog("WARN", fmt.Sprintf("API probe %s: %s%s failed after %dms: %s", r.Endpoint, response, r.FailureStage, r.Latency.Milliseconds(), r.Error))
			continue
		}
		level := "INFO"
		if r.StatusCode >= 500 || strings.Contains(strings.ToLower(r.APIMessage), "location is not supported") {
			level = "WARN"
		}
		h.console.AddLog(level, fmt.Sprintf("API probe %s: HTTP %d; proxy TCP %dms, TLS %dms, first byte %dms, total %dms; transport reachable, agy eligibility unverified", r.Endpoint, r.StatusCode, r.ProxyTCP.Milliseconds(), r.TLS.Milliseconds(), r.FirstByte.Milliseconds(), r.Latency.Milliseconds()))
		if r.APIReason != "" {
			h.console.AddLog(level, fmt.Sprintf("API metadata: reason=%s service=%s method=%s", r.APIReason, r.APIService, r.APIMethod))
		}

		if r.APIStatus != "" {
			h.console.AddLog(level, fmt.Sprintf("API response: %s: %s", r.APIStatus, r.APIMessage))
		}
	}
}
