// Package proxymode defines the protocol used to connect to the local WARP proxy.
package proxymode

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type Mode string

const (
	SOCKS5 Mode = "socks5"
	HTTP   Mode = "http"
)

func (m Mode) Valid() bool { return m == SOCKS5 || m == HTTP }
func (m Mode) Label() string {
	if m == HTTP {
		return "HTTP CONNECT"
	}
	return "SOCKS5"
}

// Selection is safe to read while background checks are running. Its zero value is SOCKS5.
type Selection struct{ value atomic.Value }

func (s *Selection) Get() Mode {
	if v := s.value.Load(); v != nil {
		return v.(Mode)
	}
	return SOCKS5
}
func (s *Selection) Set(m Mode) {
	if m.Valid() {
		s.value.Store(m)
	}
}
func (m Mode) URL(addr string) (*url.URL, error) {
	if addr == "" {
		addr = "127.0.0.1:40000"
	}
	if !strings.Contains(addr, "://") {
		addr = string(m) + "://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "socks5" && u.Scheme != "socks5h" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("an explicit SOCKS5 or HTTP proxy address is required")
	}
	return u, nil
}
func SettingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "agywarp", "settings.json"), err
}
func Load(path string) (Mode, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SOCKS5, nil
	}
	if err != nil {
		return SOCKS5, err
	}
	var settings struct {
		Mode Mode `json:"proxy_mode"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return SOCKS5, err
	}
	if !settings.Mode.Valid() {
		return SOCKS5, fmt.Errorf("invalid proxy_mode %q", settings.Mode)
	}
	return settings.Mode, nil
}
func Save(path string, mode Mode) error {
	if !mode.Valid() {
		return fmt.Errorf("invalid proxy mode %q", mode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	data, _ := json.Marshal(struct {
		Mode Mode `json:"proxy_mode"`
	}{mode})
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
