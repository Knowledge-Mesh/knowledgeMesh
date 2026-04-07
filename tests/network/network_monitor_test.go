package network_test

import (
	"context"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
)

func TestClassifyFingerprintChange(t *testing.T) {
	t.Parallel()

	before := "en0|192.168.1.2"
	after := "pdp_ip0|10.0.0.12"
	ev := network.ClassifyFingerprintChange(before, after)
	if !ev.IPChanged {
		t.Fatal("expected IPChanged=true")
	}
	if !ev.InterfaceChanged {
		t.Fatal("expected InterfaceChanged=true")
	}
}

func TestSplitNetworkFingerprint(t *testing.T) {
	t.Parallel()

	ifaces, ips := network.SplitNetworkFingerprint("en0,wlan0|192.168.1.2,10.0.0.2")
	if ifaces != "en0,wlan0" || ips != "192.168.1.2,10.0.0.2" {
		t.Fatalf("unexpected split: ifaces=%q ips=%q", ifaces, ips)
	}
}

func TestLocalNetworkFingerprintCallable(t *testing.T) {
	t.Parallel()

	// In sandboxed CI/IDE environments, route/interface reads can be restricted.
	_, _ = network.LocalNetworkFingerprint()
}

func TestNetworkMonitorNotifyNetworkChange(t *testing.T) {
	t.Parallel()

	m := network.NewNetworkMonitor(nil, nil, nil, network.DefaultNetworkMonitorConfig())
	var auto, adv bool
	m.OnAutoNATRefresh = func(network.NetworkChangeEvent) { auto = true }
	m.OnReAdvertise = func(network.NetworkChangeEvent) { adv = true }

	m.NotifyNetworkChange(context.Background(), network.NetworkChangeEvent{IPChanged: true, InterfaceChanged: true})

	if !auto {
		t.Fatal("expected OnAutoNATRefresh callback")
	}
	if !adv {
		t.Fatal("expected OnReAdvertise callback")
	}
}
