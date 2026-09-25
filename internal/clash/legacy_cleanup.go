package clash

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var profileID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// cleanLegacyWARP removes only legacy agywarp rules and its local proxy from
// subscription enhancement files. Current runtime injection never writes them.
func (m *SystemdManager) cleanLegacyWARP() error {
	data, err := os.ReadFile(filepath.Join(m.BaseDir, "profiles.yaml"))
	if err != nil {
		return fmt.Errorf("read profile index: %w", err)
	}
	var profiles VergeProfiles
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return fmt.Errorf("parse profile index: %w", err)
	}
	seen := make(map[string]bool)
	for _, item := range profiles.Items {
		if item.Option == nil {
			continue
		}
		for _, ref := range []struct{ id, kind string }{{item.Option.Rules, "rules"}, {item.Option.Proxies, "proxies"}} {
			if ref.id == "" {
				continue
			}
			if !profileID.MatchString(ref.id) {
				return fmt.Errorf("invalid profile enhancement ID %q", ref.id)
			}
			path := filepath.Join(m.BaseDir, "profiles", ref.id+".yaml")
			if seen[path] {
				continue
			}
			seen[path] = true
			if err := cleanLegacyFile(path, ref.kind); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanLegacyFile(path, kind string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid enhancement %s", path)
	}
	root := doc.Content[0]
	changed := false
	for _, key := range []string{"prepend", "append"} {
		list := field(root, key)
		if list == nil || list.Kind != yaml.SequenceNode {
			continue
		}
		kept := make([]*yaml.Node, 0, len(list.Content))
		for _, node := range list.Content {
			remove := false
			if kind == "rules" && node.Kind == yaml.ScalarNode {
				rule := strings.TrimSpace(node.Value)
				remove = strings.HasPrefix(rule, "PROCESS-NAME,warp-svc,") || strings.HasSuffix(rule, ",WARP-LOCAL") || strings.HasSuffix(rule, ","+runtimeProxy)
			}
			if kind == "proxies" && node.Kind == yaml.MappingNode {
				name, server, typ, port := field(node, "name"), field(node, "server"), field(node, "type"), field(node, "port")
				remove = name != nil && server != nil && typ != nil && port != nil && name.Value == "WARP-LOCAL" && server.Value == "127.0.0.1" && typ.Value == "socks5" && port.Value == "40000"
			}
			if remove {
				changed = true
			} else {
				kept = append(kept, node)
			}
		}
		list.Content = kept
	}
	if !changed {
		return nil
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agywarp-clean-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
