package checker

import (
	"context"
	"testing"
	"time"
)

func TestParseTraceResponseBody(t *testing.T) {
	mockBody := `fl=1164f53
h=1.1.1.1
ip=104.28.195.192
ts=1790245525.000
visit_scheme=https
uag=curl/8.22.0
colo=LAX
sliver=none
http=http/2
loc=US
tls=TLSv1.3
sni=off
warp=on
gateway=off
rbi=off
`
	verif := ParseTraceResponseBody(mockBody)
	if !verif.IsWarp {
		t.Errorf("expected IsWarp=true")
	}
	if verif.IP != "104.28.195.192" {
		t.Errorf("expected IP=104.28.195.192, got %s", verif.IP)
	}
	if verif.Loc != "US" {
		t.Errorf("expected Loc=US, got %s", verif.Loc)
	}
	if verif.Colo != "LAX" {
		t.Errorf("expected Colo=LAX, got %s", verif.Colo)
	}
	if verif.Endpoint != "1.1.1.1" {
		t.Errorf("expected Endpoint=1.1.1.1, got %s", verif.Endpoint)
	}
}

func TestCheckPortListening(t *testing.T) {
	c := NewChecker()
	// Test port probe on active local port (e.g. 40000)
	isListening := c.CheckPortListening(context.Background(), "127.0.0.1:40000")
	t.Logf("Port 40000 listening: %v", isListening)

	// Test non-listening port
	notListening := c.CheckPortListening(context.Background(), "127.0.0.1:54321")
	if notListening {
		t.Errorf("expected port 54321 to not be listening")
	}
}

func TestVerifyExitRealIfListening(t *testing.T) {
	c := NewChecker()
	if !c.CheckPortListening(context.Background(), "127.0.0.1:40000") {
		t.Skip("skipping real trace test: port 40000 is not listening")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	verif, err := c.VerifyExit(ctx, "127.0.0.1:40000")
	if err != nil {
		t.Logf("VerifyExit error (might be offline): %v", err)
		return
	}

	t.Logf("Live WARP Trace: isWarp=%v, IP=%s, Loc=%s, Colo=%s, Latency=%v",
		verif.IsWarp, verif.IP, verif.Loc, verif.Colo, verif.Latency)
	if verif.IP == "" {
		t.Errorf("expected non-empty IP from live trace")
	}
}
