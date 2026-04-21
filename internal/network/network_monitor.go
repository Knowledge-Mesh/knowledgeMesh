package network

import (
	"context"
	"log"
	"net"
	"sort"
	"strings"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
)

const defaultNetworkPollInterval = 3 * time.Second

type NetworkMonitorConfig struct {
	PollInterval time.Duration
}

func DefaultNetworkMonitorConfig() NetworkMonitorConfig {
	return NetworkMonitorConfig{PollInterval: defaultNetworkPollInterval}
}

func normalizeNetworkMonitorConfig(cfg NetworkMonitorConfig) NetworkMonitorConfig {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultNetworkPollInterval
	}
	return cfg
}

type NetworkChangeEvent struct {
	IPChanged        bool
	InterfaceChanged bool
	BeforeFingerprint string
	AfterFingerprint  string
}

// NetworkMonitor polls local interfaces and addresses to detect unstable/mobile
// network transitions (IP change, WiFi/cellular switch) and emits callbacks to
// p2p subsystems that need re-evaluation.
type NetworkMonitor struct {
	h          host.Host
	holePunch  *HolePunchManager
	connType   *ConnectionTypeTracker
	cfg        NetworkMonitorConfig

	// Optional hooks for integration points.
	OnAutoNATRefresh func(NetworkChangeEvent)
	OnReAdvertise    func(NetworkChangeEvent)
}

func NewNetworkMonitor(h host.Host, hp *HolePunchManager, ct *ConnectionTypeTracker, cfg NetworkMonitorConfig) *NetworkMonitor {
	return &NetworkMonitor{
		h:         h,
		holePunch: hp,
		connType:  ct,
		cfg:       normalizeNetworkMonitorConfig(cfg),
	}
}

func (m *NetworkMonitor) Start(ctx context.Context) {
	prev, err := localNetworkFingerprint()
	if err != nil {
		log.Printf("[netmon] initial fingerprint error: %v", err)
		prev = ""
	}

	ticker := time.NewTicker(m.cfg.PollInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cur, err := localNetworkFingerprint()
				if err != nil {
					log.Printf("[netmon] fingerprint error: %v", err)
					continue
				}
				if cur == prev {
					continue
				}
				ev := ClassifyFingerprintChange(prev, cur)
				prev = cur
				m.handleChange(ctx, ev)
			}
		}
	}()
}

// NotifyNetworkChange runs the same reconciliation as when the monitor detects an OS-level change.
func (m *NetworkMonitor) NotifyNetworkChange(ctx context.Context, ev NetworkChangeEvent) {
	m.handleChange(ctx, ev)
}

func (m *NetworkMonitor) handleChange(ctx context.Context, ev NetworkChangeEvent) {
	log.Printf("[netmon] network change detected ip_changed=%v interface_changed=%v", ev.IPChanged, ev.InterfaceChanged)

	// Prompt libp2p connection manager to re-evaluate under new network conditions.
	if m.h != nil {
		m.h.ConnManager().TrimOpenConns(ctx)
	}

	// Nudge AutoNAT dependent paths via hook (libp2p auto services continue in background).
	if m.OnAutoNATRefresh != nil {
		m.OnAutoNATRefresh(ev)
	}

	// Restart hole punch workers for active relay-only peers.
	if m.holePunch != nil {
		m.holePunch.HandleNetworkChange("network_monitor_change")
	}

	// Re-tag/refresh connection classification for routing.
	if m.connType != nil {
		m.connType.HandleNetworkChange()
	}

	// Allow caller to re-advertise presence/listen addresses.
	if m.OnReAdvertise != nil {
		m.OnReAdvertise(ev)
	}
}

// ClassifyFingerprintChange compares two fingerprints from LocalNetworkFingerprint.
func ClassifyFingerprintChange(before, after string) NetworkChangeEvent {
	oldIfaces, oldIPs := SplitNetworkFingerprint(before)
	newIfaces, newIPs := SplitNetworkFingerprint(after)
	return NetworkChangeEvent{
		IPChanged:         oldIPs != newIPs,
		InterfaceChanged:  oldIfaces != newIfaces,
		BeforeFingerprint: before,
		AfterFingerprint:  after,
	}
}

// SplitNetworkFingerprint splits a fingerprint string into interface and IP parts.
func SplitNetworkFingerprint(fp string) (ifaces string, ips string) {
	parts := strings.SplitN(fp, "|", 2)
	if len(parts) != 2 {
		return fp, fp
	}
	return parts[0], parts[1]
}

// LocalNetworkFingerprint returns a stable string of non-loopback interface names and IPs.
func LocalNetworkFingerprint() (string, error) {
	return localNetworkFingerprint()
}

func localNetworkFingerprint() (string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var ifaceNames []string
	var ips []string
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifaceNames = append(ifaceNames, ifc.Name)
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil || ip == nil {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	sort.Strings(ifaceNames)
	sort.Strings(ips)
	return strings.Join(ifaceNames, ",") + "|" + strings.Join(ips, ","), nil
}

