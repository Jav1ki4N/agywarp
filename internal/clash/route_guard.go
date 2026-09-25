package clash

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func hashBase(base []byte) string {
	h := sha256.Sum256(base)
	return hex.EncodeToString(h[:])
}

type routeSnapshot struct {
	ProfileUID string            `json:"profile_uid,omitempty"`
	Selectors  map[string]string `json:"selectors,omitempty"`
}

func (m *SystemdManager) currentRoute(ctx context.Context) (routeSnapshot, error) {
	var result routeSnapshot
	data, err := os.ReadFile(filepath.Join(m.BaseDir, "profiles.yaml"))
	if err != nil {
		return result, err
	}
	var profiles VergeProfiles
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return result, err
	}
	result.ProfileUID = profiles.Current
	if result.ProfileUID == "" {
		return result, fmt.Errorf("no selected Clash Verge profile")
	}
	var response struct {
		Proxies map[string]struct {
			Type string `json:"type"`
			Now  string `json:"now"`
		} `json:"proxies"`
	}
	if err := m.getJSON(ctx, "/proxies", &response); err != nil {
		return result, err
	}
	result.Selectors = make(map[string]string)
	for name, proxy := range response.Proxies {
		if strings.EqualFold(proxy.Type, "Selector") {
			result.Selectors[name] = proxy.Now
		}
	}
	return result, nil
}

// RuntimeRouteChanged reports external profile or selector changes while ON.
// A missing snapshot denotes a session created by an older agywarp version.
func (m *SystemdManager) RuntimeRouteChanged(ctx context.Context) (string, error) {
	data, err := os.ReadFile(m.sessionPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var session runtimeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return "", err
	}
	base, err := os.ReadFile(m.basePath())
	if err != nil {
		return "", err
	}
	if hashBase(base) != session.BaseHash {
		return "generated Mihomo config changed", nil
	}
	if session.Route.ProfileUID == "" {
		return "", nil
	}
	current, err := m.currentRoute(ctx)
	if err != nil {
		return "", err
	}
	if current.ProfileUID != session.Route.ProfileUID {
		return "airport profile changed", nil
	}
	for name, choice := range session.Route.Selectors {
		if current.Selectors[name] != choice {
			return fmt.Sprintf("node selection changed in %s", name), nil
		}
	}
	return "", nil
}
