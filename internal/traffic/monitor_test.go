package traffic

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxMonitorWithMockFiles(t *testing.T) {
	tmpDir := t.TempDir()
	devPath := filepath.Join(tmpDir, "dev")
	routePath := filepath.Join(tmpDir, "route")

	// Sample 1 mock data
	mockDev1 := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000 10 0 0 0 0 0 0 1000 10 0 0 0 0 0 0
wlp3s0: 100000 500 0 0 0 0 0 0 50000 400 0 0 0 0 0 0
Mihomo: 20000 100 0 0 0 0 0 0 10000 80 0 0 0 0 0 0
`
	mockRoute := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlp3s0	00000000	0132A8C0	0003	0	0	20600	00000000	0	0	0
wlp3s0	0032A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
`
	if err := os.WriteFile(devPath, []byte(mockDev1), 0644); err != nil {
		t.Fatalf("writing mock dev: %v", err)
	}
	if err := os.WriteFile(routePath, []byte(mockRoute), 0644); err != nil {
		t.Fatalf("writing mock route: %v", err)
	}

	mon := &LinuxMonitor{
		netDevPath:   devPath,
		netRoutePath: routePath,
	}

	// First sample (baseline)
	s1, err := mon.Sample()
	if err != nil {
		t.Fatalf("Sample 1 failed: %v", err)
	}
	if s1.InterfaceName != "wlp3s0" {
		t.Errorf("expected default interface wlp3s0, got %s", s1.InterfaceName)
	}
	if s1.IsProxy {
		t.Errorf("expected isProxy=false for default route")
	}
	if s1.DownSpeed != 0 || s1.UpSpeed != 0 {
		t.Errorf("expected initial speed 0, got down=%f up=%f", s1.DownSpeed, s1.UpSpeed)
	}

	// Update mock dev to simulate 1 second of traffic:
	// +10,000 bytes down, +5,000 bytes up on wlp3s0
	mockDev2 := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000 10 0 0 0 0 0 0 1000 10 0 0 0 0 0 0
wlp3s0: 110000 550 0 0 0 0 0 0 55000 450 0 0 0 0 0 0
Mihomo: 20000 100 0 0 0 0 0 0 10000 80 0 0 0 0 0 0
`
	if err := os.WriteFile(devPath, []byte(mockDev2), 0644); err != nil {
		t.Fatalf("writing mock dev 2: %v", err)
	}

	// Artificially step back lastTime by 1 second for deterministic test
	mon.mu.Lock()
	mon.lastTime = mon.lastTime.Add(-1 * time.Second)
	mon.mu.Unlock()

	s2, err := mon.Sample()
	if err != nil {
		t.Fatalf("Sample 2 failed: %v", err)
	}

	// delta was 10,000 down and 5,000 up over ~1 second
	if s2.DownSpeed < 9900 || s2.DownSpeed > 10100 {
		t.Errorf("expected DownSpeed ~10000, got %f", s2.DownSpeed)
	}
	if s2.UpSpeed < 4900 || s2.UpSpeed > 5100 {
		t.Errorf("expected UpSpeed ~5000, got %f", s2.UpSpeed)
	}

	// Test Proxy Interface override
	mon.SetProxyInterface("Mihomo")
	s3, err := mon.Sample()
	if err != nil {
		t.Fatalf("Sample 3 failed: %v", err)
	}
	if s3.InterfaceName != "Mihomo" {
		t.Errorf("expected interface Mihomo, got %s", s3.InterfaceName)
	}
	if !s3.IsProxy {
		t.Errorf("expected isProxy=true for Mihomo")
	}
}
