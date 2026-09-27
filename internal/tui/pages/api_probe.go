package pages

import (
	"fmt"
	"net/url"
	"strings"

	"agywarp/internal/checker"
)

func probeTarget(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}
	action := u.Path
	if i := strings.LastIndex(action, ":"); i >= 0 {
		action = action[i+1:]
	} else if i := strings.LastIndex(action, "/"); i >= 0 {
		action = action[i+1:]
	}
	if action == "" {
		return u.Host
	}
	return u.Host + "/" + action
}

func probeFailure(r checker.APIProbeResult) string {
	// Strip repeated request URLs only for known transport failures. Preserve
	// unfamiliar errors so an actionable cause is not accidentally hidden.
	for _, reason := range []string{"host unreachable", "network unreachable", "connection refused", "Bad Gateway", "context deadline exceeded", "TLS handshake timeout"} {
		if strings.Contains(r.Error, reason) {
			return reason
		}
	}
	return strings.ReplaceAll(r.Error, r.Endpoint, probeTarget(r.Endpoint))
}

func (h *Home) showAPIProbes(results []checker.APIProbeResult) {
	shown := make([]bool, len(results))
	for i, r := range results {
		if shown[i] {
			continue
		}
		if r.Error != "" {
			reason := probeFailure(r)
			targets := []string{probeTarget(r.Endpoint)}
			minMS, maxMS := r.Latency.Milliseconds(), r.Latency.Milliseconds()
			for j := i + 1; j < len(results); j++ {
				other := results[j]
				if !shown[j] && other.Error != "" && other.FailureStage == r.FailureStage && other.StatusCode == r.StatusCode && probeFailure(other) == reason {
					shown[j] = true
					targets = append(targets, probeTarget(other.Endpoint))
					minMS = min(minMS, other.Latency.Milliseconds())
					maxMS = max(maxMS, other.Latency.Milliseconds())
				}
			}
			label := "API " + targets[0]
			if len(targets) > 1 {
				label = fmt.Sprintf("API probes %d/%d", len(targets), len(results))
				if len(targets) != len(results) {
					label += " (" + strings.Join(targets, ", ") + ")"
				}
			}
			duration := fmt.Sprintf("%dms", minMS)
			if minMS != maxMS {
				duration = fmt.Sprintf("%d–%dms", minMS, maxMS)
			}
			response := ""
			if r.StatusCode != 0 {
				response = fmt.Sprintf(" · HTTP %d received", r.StatusCode)
			}
			h.console.AddLog("WARN", fmt.Sprintf("%s%s · %s failed · %s · %s", label, response, r.FailureStage, duration, reason))
			continue
		}
		level := "INFO"
		if r.StatusCode >= 500 || strings.Contains(strings.ToLower(r.APIMessage), "location is not supported") {
			level = "WARN"
		}
		details := []string{fmt.Sprintf("API %s", probeTarget(r.Endpoint)), fmt.Sprintf("HTTP %d (unauthenticated)", r.StatusCode), fmt.Sprintf("%dms", r.Latency.Milliseconds())}
		if r.APIStatus != "" {
			details = append(details, r.APIStatus)
		}
		if r.APIReason != "" {
			details = append(details, r.APIReason)
		}
		if r.APIMessage != "" {
			message := []rune(r.APIMessage)
			if len(message) > 160 {
				message = append(message[:160], '…')
			}
			details = append(details, string(message))
		}
		h.console.AddLog(level, strings.Join(details, " · "))
	}
}
