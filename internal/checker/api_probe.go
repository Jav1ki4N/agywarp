package checker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
)

// APIProbeResult reports transport and HTTP response evidence, never business eligibility.
type APIProbeResult struct {
	Endpoint     string
	Method       string
	APIReason    string
	APIService   string
	APIMethod    string
	StatusCode   int
	Latency      time.Duration
	ProxyTCP     time.Duration
	TLS          time.Duration
	FirstByte    time.Duration
	FailureStage string
	Error        string
	APIStatus    string
	APIMessage   string
}

type APIProber interface {
	ProbeAPIs(context.Context, string) []APIProbeResult
}

// These requests are intentionally unauthenticated; they test method reachability,
// not authenticated Antigravity eligibility.
var defaultAPIProbeURLs = []string{
	"https://generativelanguage.googleapis.com/v1beta/models",
	"https://cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels",
	"https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels",
}

func (c *HTTPChecker) ProbeAPIs(ctx context.Context, socks5Addr string) []APIProbeResult {
	targets := c.ProbeURLs
	if targets == nil {
		targets = defaultAPIProbeURLs
	}
	proxyURL, proxyErr := c.proxyMode.Get().URL(socks5Addr)
	results := make([]APIProbeResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target string) {
			defer wg.Done()
			if proxyErr != nil {
				results[i] = APIProbeResult{Endpoint: target, FailureStage: "proxy configuration", Error: proxyErr.Error()}
				return
			}
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			results[i] = probeAPI(ctx, client, target)
		}(i, target)
	}
	wg.Wait()
	return results
}

func probeAPI(ctx context.Context, client *http.Client, target string) APIProbeResult {
	method := http.MethodGet
	var bodyReader io.Reader
	if u, err := url.Parse(target); err == nil && u.Path == "/v1internal:fetchAvailableModels" {
		method = http.MethodPost
		bodyReader = strings.NewReader("{}")
	}
	r := APIProbeResult{Endpoint: target, Method: method}
	start := time.Now()
	var mu sync.Mutex
	stage := "proxy TCP connection"
	targetStage := "SOCKS/target connection"
	if client.Transport != nil {
		if transport, ok := client.Transport.(*http.Transport); ok && transport.Proxy != nil {
			req, _ := http.NewRequest(http.MethodGet, target, nil)
			if req != nil {
				if proxy, err := transport.Proxy(req); err == nil && proxy != nil && proxy.Scheme == "http" {
					targetStage = "HTTP CONNECT/target connection"
				}
			}
		}
	}
	var tcpStart, tlsStart time.Time
	trace := &httptrace.ClientTrace{
		ConnectStart: func(_, _ string) { mu.Lock(); tcpStart = time.Now(); mu.Unlock() },
		ConnectDone: func(_, _ string, err error) {
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				r.ProxyTCP = time.Since(tcpStart)
				stage = targetStage
			}
		},
		TLSHandshakeStart: func() { mu.Lock(); tlsStart = time.Now(); stage = "TLS handshake"; mu.Unlock() },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			mu.Lock()
			defer mu.Unlock()
			r.TLS = time.Since(tlsStart)
			if err == nil {
				stage = "HTTP response"
			}
		},
		GotFirstResponseByte: func() { mu.Lock(); r.FirstByte = time.Since(start); mu.Unlock() },
	}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, target, bodyReader)
	if err != nil {
		r.FailureStage = "request configuration"
		r.Error = err.Error()
		return r
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	mu.Lock()
	snapshot := r
	snapshot.Latency = time.Since(start)
	if err != nil {
		snapshot.FailureStage = stage
		snapshot.Error = err.Error()
	}
	mu.Unlock()
	if err != nil {
		return snapshot
	}
	defer response.Body.Close()
	snapshot.StatusCode = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	snapshot.Latency = time.Since(start)
	if err != nil {
		snapshot.FailureStage = "response body"
		snapshot.Error = err.Error()
		return snapshot
	}
	var apiError struct {
		Error struct {
			Status  string
			Message string
			Details []struct {
				Type     string `json:"@type"`
				Reason   string
				Metadata struct {
					Service string
					Method  string
				}
			}
		}
	}
	if json.Unmarshal(body, &apiError) == nil {
		snapshot.APIStatus = apiError.Error.Status
		snapshot.APIMessage = apiError.Error.Message
		for _, detail := range apiError.Error.Details {
			if detail.Type == "type.googleapis.com/google.rpc.ErrorInfo" {
				snapshot.APIReason = detail.Reason
				snapshot.APIService = detail.Metadata.Service
				snapshot.APIMethod = detail.Metadata.Method
				break
			}
		}
	}
	return snapshot
}
