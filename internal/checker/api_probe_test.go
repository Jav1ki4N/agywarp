package checker

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
)

type probeRoundTripper func(*http.Request) (*http.Response, error)

func (f probeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAPIProbeHTTPDenialIsResponseEvidence(t *testing.T) {
	client := &http.Client{Transport: probeRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" || req.Header.Get("x-goog-api-key") != "" {
			t.Fatal("probe must be unauthenticated")
		}
		trace := httptrace.ContextClientTrace(req.Context())
		trace.ConnectStart("tcp", "127.0.0.1:40000")
		trace.ConnectDone("tcp", "127.0.0.1:40000", nil)
		trace.TLSHandshakeStart()
		trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
		trace.GotFirstResponseByte()
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"PERMISSION_DENIED","message":"Unregistered caller"}}`)), Header: make(http.Header)}, nil
	})}
	result := probeAPI(context.Background(), client, "https://generativelanguage.googleapis.com/v1beta/models")
	if result.StatusCode != 403 || result.Error != "" || result.APIStatus != "PERMISSION_DENIED" || result.FirstByte == 0 {
		t.Fatalf("unexpected probe: %+v", result)
	}
}

func TestAPIProbeTLSFailureAndResponseTimeout(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		completeTLS bool
	}{
		{"TLS failure", "TLS handshake", false},
		{"response timeout", "HTTP response", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: probeRoundTripper(func(req *http.Request) (*http.Response, error) {
				trace := httptrace.ContextClientTrace(req.Context())
				trace.TLSHandshakeStart()
				if tc.completeTLS {
					trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
				}
				return nil, context.DeadlineExceeded
			})}
			r := probeAPI(context.Background(), client, "https://cloudcode-pa.googleapis.com/")
			if r.FailureStage != tc.stage || r.StatusCode != 0 || !strings.Contains(r.Error, context.DeadlineExceeded.Error()) {
				t.Fatalf("probe=%+v", r)
			}
		})
	}
}

func TestAPIProbeRegionMessagePreserved(t *testing.T) {
	client := &http.Client{Transport: probeRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"FAILED_PRECONDITION","message":"User location is not supported for the API use."}}`)), Header: make(http.Header)}, nil
	})}
	r := probeAPI(context.Background(), client, "https://generativelanguage.googleapis.com/v1beta/models")
	if r.APIStatus != "FAILED_PRECONDITION" || !strings.Contains(r.APIMessage, "location is not supported") {
		t.Fatalf("probe=%+v", r)
	}
}

func TestAPIProbeRejectsUnsupportedProxy(t *testing.T) {
	c := &HTTPChecker{ProbeURLs: []string{"https://cloudcode-pa.googleapis.com/"}}
	result := c.ProbeAPIs(context.Background(), "https://127.0.0.1:7890")
	if len(result) != 1 || result[0].FailureStage != "proxy configuration" || result[0].Error == "" {
		t.Fatalf("probe=%+v", result)
	}
}

func TestAPIProbeBodyFailureStillRecordsHTTPStatus(t *testing.T) {
	client := &http.Client{Transport: probeRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: brokenProbeBody{}, Header: make(http.Header)}, nil
	})}
	r := probeAPI(context.Background(), client, "https://cloudcode-pa.googleapis.com/")
	if r.StatusCode != 403 || r.FailureStage != "response body" {
		t.Fatalf("probe=%+v", r)
	}
}

type brokenProbeBody struct{}

func TestAntigravityMethodProbe(t *testing.T) {
	client := &http.Client{Transport: probeRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if req.Method != "POST" || string(body) != "{}" || req.Header.Get("Content-Type") != "application/json" || req.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected method probe: %s %s %v", req.Method, body, req.Header)
		}
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"UNAUTHENTICATED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"CREDENTIALS_MISSING","metadata":{"service":"cloudcode-pa.googleapis.com","method":"FetchAvailableModels"}}]}}`)), Header: make(http.Header)}, nil
	})}
	r := probeAPI(context.Background(), client, "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels")
	if r.Method != "POST" || r.StatusCode != 401 || r.APIReason != "CREDENTIALS_MISSING" || r.APIService != "cloudcode-pa.googleapis.com" || r.APIMethod != "FetchAvailableModels" {
		t.Fatalf("probe=%+v", r)
	}
}

func (brokenProbeBody) Read([]byte) (int, error) { return 0, errors.New("body interrupted") }
func (brokenProbeBody) Close() error             { return nil }
