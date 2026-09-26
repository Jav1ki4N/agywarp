package proxymode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agywarp", "settings.json")
	if mode, err := Load(path); err != nil || mode != SOCKS5 {
		t.Fatalf("%s %v", mode, err)
	}
	for _, mode := range []Mode{HTTP, SOCKS5} {
		if err := Save(path, mode); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(path); err != nil || got != mode {
			t.Fatalf("%s %v", got, err)
		}
	}
	if err := Save(path, "invalid"); err == nil {
		t.Fatal("accepted invalid preference")
	}
	if got, _ := Load(path); got != SOCKS5 {
		t.Fatal("invalid save replaced preference")
	}
	if err := os.WriteFile(path, []byte(`{"proxy_mode":"direct"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted invalid saved mode")
	}
}
