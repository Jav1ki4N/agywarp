package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"agywarp/internal/clash"
	"agywarp/internal/process"
	"agywarp/internal/warp"
)

// preflight runs before entering the alternate screen. It has no network or config side effects.
func preflight(ctx context.Context, out io.Writer) (clash.RuntimeCheck, error) {
	log := func(level, format string, args ...any) {
		fmt.Fprintf(out, "[%s] %s\n", level, fmt.Sprintf(format, args...))
	}
	log("INFO", "agywarp preflight")
	if runtime.GOOS != "linux" {
		return clash.RuntimeCheck{}, fmt.Errorf("process discovery requires Linux")
	}
	log("OK", "Linux process discovery available")
	store, err := process.NewStore()
	if err != nil {
		return clash.RuntimeCheck{}, err
	}
	profiles := []process.Profile{}
	if data, err := os.ReadFile(store.Path); err == nil {
		if err := json.Unmarshal(data, &profiles); err != nil {
			return clash.RuntimeCheck{}, fmt.Errorf("parse %s: %w", store.Path, err)
		}
	} else if !os.IsNotExist(err) {
		return clash.RuntimeCheck{}, fmt.Errorf("read %s: %w", store.Path, err)
	}
	log("OK", "Loaded %d process profiles", len(profiles))
	manager := clash.NewManager()
	check, err := manager.Preflight(ctx)
	if err != nil {
		return check, err
	}
	var rules []string
	for _, p := range profiles {
		rules = append(rules, p.ToProcessRules("AGYWARP-WARP")...)
	}
	if len(rules) > 0 {
		base, err := os.ReadFile(check.BasePath)
		if err != nil {
			return check, err
		}
		if _, err := clash.BuildRuntime(base, rules, 40000); err != nil {
			return check, fmt.Errorf("runtime dry run: %w", err)
		}
	}
	if check.Active {
		desired := make(map[string]bool)
		for _, rule := range rules {
			desired[rule] = true
		}
		for _, rule := range check.Rules {
			if !desired[rule] {
				return check, fmt.Errorf("profile JSON differs from applied runtime rules")
			}
			delete(desired, rule)
		}
		if len(desired) > 0 {
			return check, fmt.Errorf("profile JSON differs from applied runtime rules")
		}
	}
	log("OK", "Mihomo %s via %s", check.Version, check.SocketPath)
	log("OK", "TUN and process matching enabled; base config clean")
	client := warp.NewClient()
	install, err := client.Detect(ctx)
	if err != nil {
		return check, err
	}
	if !install.Installed {
		return check, fmt.Errorf("warp-cli is not installed")
	}
	status, err := client.Status(ctx)
	if err != nil {
		return check, fmt.Errorf("warp-svc unavailable: %w", err)
	}
	if status.Mode != "WarpProxy" || status.ProxyPort < 1 {
		return check, fmt.Errorf("WARP must use WarpProxy mode with a valid port; got %q port %d", status.Mode, status.ProxyPort)
	}
	log("OK", "WARP %s; WarpProxy on port %d", status.Status, status.ProxyPort)
	if status.Status != "CONNECTED" {
		log("INFO", "WARP is disconnected; Network Card will connect it when started")
	}
	if check.Active {
		log("OK", "Existing agywarp runtime detected (%d rules)", len(check.Rules))
	}
	log("OK", "All required checks passed. Entering dashboard...")
	return check, nil
}

func runPreflight(out io.Writer) (clash.RuntimeCheck, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	check, err := preflight(ctx, out)
	if err != nil {
		fmt.Fprintf(out, "[FATAL] %v\n", err)
	}
	return check, err
}
