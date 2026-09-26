package checker

import (
	"agywarp/internal/proxymode"
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExitVerification holds the result of inspecting the outbound WARP proxy exit.
type ExitVerification struct {
	IsWarp    bool             `json:"is_warp"`
	Loc       string           `json:"loc"`
	IP        string           `json:"ip"`
	Colo      string           `json:"colo"`
	Latency   time.Duration    `json:"latency"`
	Endpoint  string           `json:"endpoint"`
	APIProbes []APIProbeResult `json:"api_probes,omitempty"`
}

// Checker verifies connectivity through the local WARP proxy.
type Checker interface {
	VerifyExit(ctx context.Context, socks5Addr string) (*ExitVerification, error)
	CheckPortListening(ctx context.Context, addr string) bool
}

// HTTPChecker performs verification via Cloudflare trace endpoint.
type HTTPChecker struct {
	proxyMode proxymode.Selection
	TraceURL  string
	ProbeURLs []string
}

// NewChecker constructs a new HTTPChecker instance.
func NewChecker() Checker {
	return &HTTPChecker{
		TraceURL: "https://1.1.1.1/cdn-cgi/trace",
	}
}

// CheckPortListening probes whether an address (e.g. "127.0.0.1:40000") is accepting TCP connections.
func (c *HTTPChecker) CheckPortListening(ctx context.Context, addr string) bool {
	var d net.Dialer
	dialCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ParseTraceResponseBody parses the key=value format of /cdn-cgi/trace.
func ParseTraceResponseBody(body string) *ExitVerification {
	verif := &ExitVerification{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "warp":
			verif.IsWarp = (val == "on")
		case "ip":
			verif.IP = val
		case "loc":
			verif.Loc = val
		case "colo":
			verif.Colo = val
		case "h":
			verif.Endpoint = val
		}
	}
	return verif
}

// VerifyExit queries the Cloudflare trace endpoint via the selected local proxy and parses output.
func (c *HTTPChecker) VerifyExit(ctx context.Context, socks5Addr string) (*ExitVerification, error) {
	proxyURL, err := c.proxyMode.Get().URL(socks5Addr)
	if err != nil {
		return nil, fmt.Errorf("parsing proxy URL: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // match legacy --insecure behavior
		},
		DisableKeepAlives: true,
		OnProxyConnectResponse: func(_ context.Context, proxy *url.URL, req *http.Request, resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP CONNECT proxy %s rejected target %s: %s", proxy.Host, req.Host, resp.Status)
			}
			return nil
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   6 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.TraceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating trace request: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing trace request: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}

	verif := ParseTraceResponseBody(sb.String())
	verif.Latency = latency

	return verif, nil
}

func (c *HTTPChecker) SetProxyMode(mode proxymode.Mode) { c.proxyMode.Set(mode) }
