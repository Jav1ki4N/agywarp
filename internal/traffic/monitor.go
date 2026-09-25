package traffic

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sample represents a point-in-time network traffic measurement.
type Sample struct {
	Timestamp     time.Time
	DownBytes     uint64
	UpBytes       uint64
	DownSpeed     float64 // Bytes per second
	UpSpeed       float64 // Bytes per second
	TotalSpeed    float64 // DownSpeed + UpSpeed
	InterfaceName string
	IsProxy       bool
}

// Monitor collects network traffic metrics.
type Monitor interface {
	Sample() (Sample, error)
	SetProxyInterface(iface string)
	GetInterfaceName() string
}

// LinuxMonitor reads network interface counters from /proc/net/dev and /proc/net/route.
type LinuxMonitor struct {
	mu           sync.Mutex
	netDevPath   string
	netRoutePath string
	proxyIface   string

	initialized bool
	lastTime    time.Time
	lastDown    uint64
	lastUp      uint64

	currentIface string
	isProxy      bool
}

// NewLinuxMonitor creates a new LinuxMonitor with system paths.
func NewLinuxMonitor() *LinuxMonitor {
	return &LinuxMonitor{
		netDevPath:   "/proc/net/dev",
		netRoutePath: "/proc/net/route",
	}
}

// SetProxyInterface sets a specific proxy interface (e.g. "Mihomo", "warp0", "tun0").
func (m *LinuxMonitor) SetProxyInterface(iface string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyIface = iface
}

// GetInterfaceName returns the active interface being monitored.
func (m *LinuxMonitor) GetInterfaceName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentIface
}

// detectDefaultInterface finds the interface with the default route (Destination 00000000).
func (m *LinuxMonitor) detectDefaultInterface() string {
	f, err := os.Open(m.netRoutePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "00000000" {
			// Found default route
			return fields[0]
		}
	}
	return ""
}

type ifaceStats struct {
	rxBytes uint64
	txBytes uint64
}

// readDevStats reads all interface stats from /proc/net/dev.
func (m *LinuxMonitor) readDevStats() (map[string]ifaceStats, error) {
	f, err := os.Open(m.netDevPath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", m.netDevPath, err)
	}
	defer f.Close()

	stats := make(map[string]ifaceStats)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 == nil && err2 == nil {
			stats[iface] = ifaceStats{rxBytes: rx, txBytes: tx}
		}
	}

	return stats, nil
}

// Sample takes a new traffic measurement and calculates instant speed.
func (m *LinuxMonitor) Sample() (Sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	allStats, err := m.readDevStats()
	if err != nil {
		return Sample{Timestamp: now}, err
	}

	targetIface := ""
	isProxy := false

	// 1. If proxy interface is configured and present in /proc/net/dev, use it
	if m.proxyIface != "" {
		if _, ok := allStats[m.proxyIface]; ok {
			targetIface = m.proxyIface
			isProxy = true
		}
	}

	// 2. Otherwise find default route interface
	if targetIface == "" {
		defIface := m.detectDefaultInterface()
		if defIface != "" {
			if _, ok := allStats[defIface]; ok {
				targetIface = defIface
			}
		}
	}

	var currentRx, currentTx uint64

	// 3. Fallback: aggregate all non-loopback interfaces if no single target detected
	if targetIface != "" {
		s := allStats[targetIface]
		currentRx = s.rxBytes
		currentTx = s.txBytes
		m.currentIface = targetIface
		m.isProxy = isProxy
	} else {
		for iface, s := range allStats {
			if iface == "lo" {
				continue
			}
			currentRx += s.rxBytes
			currentTx += s.txBytes
		}
		m.currentIface = "all-interfaces"
		m.isProxy = false
	}

	// If first sample, record baseline and return 0 speed
	if !m.initialized {
		m.initialized = true
		m.lastTime = now
		m.lastDown = currentRx
		m.lastUp = currentTx
		return Sample{
			Timestamp:     now,
			DownBytes:     currentRx,
			UpBytes:       currentTx,
			DownSpeed:     0,
			UpSpeed:       0,
			TotalSpeed:    0,
			InterfaceName: m.currentIface,
			IsProxy:       m.isProxy,
		}, nil
	}

	elapsed := now.Sub(m.lastTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1.0
	}

	var downDelta, upDelta uint64
	if currentRx >= m.lastDown {
		downDelta = currentRx - m.lastDown
	}
	if currentTx >= m.lastUp {
		upDelta = currentTx - m.lastUp
	}

	downSpeed := float64(downDelta) / elapsed
	upSpeed := float64(upDelta) / elapsed

	m.lastTime = now
	m.lastDown = currentRx
	m.lastUp = currentTx

	return Sample{
		Timestamp:     now,
		DownBytes:     currentRx,
		UpBytes:       currentTx,
		DownSpeed:     downSpeed,
		UpSpeed:       upSpeed,
		TotalSpeed:    downSpeed + upSpeed,
		InterfaceName: m.currentIface,
		IsProxy:       m.isProxy,
	}, nil
}
