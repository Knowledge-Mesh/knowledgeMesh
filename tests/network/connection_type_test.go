package network_test

import (
	"context"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func connectForConnectionTypeTest(t *testing.T, ctx context.Context) (*network.ConnectionTypeTracker, peer.ID, func()) {
	t.Helper()

	h1, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatalf("host1 create: %v", err)
	}
	h2, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		_ = h1.Close()
		t.Fatalf("host2 create: %v", err)
	}

	addr := h2.Addrs()[0].Encapsulate(ma.StringCast("/p2p/" + h2.ID().String()))
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		_ = h2.Close()
		_ = h1.Close()
		t.Fatalf("addr info parse: %v", err)
	}
	if err := h1.Connect(ctx, *info); err != nil {
		_ = h2.Close()
		_ = h1.Close()
		t.Fatalf("connect: %v", err)
	}

	tracker := network.NewConnectionTypeTracker(h1)
	tracker.Start()

	cleanup := func() {
		tracker.Close()
		_ = h2.Close()
		_ = h1.Close()
	}
	return tracker, h2.ID(), cleanup
}

func TestConnectionTypeTrackerDetectsDirect(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tracker, target, cleanup := connectForConnectionTypeTest(t, ctx)
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for !tracker.IsDirectConnection(target) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !tracker.IsDirectConnection(target) {
		t.Fatal("expected direct connection")
	}
	if tracker.IsRelayConnection(target) {
		t.Fatal("did not expect relay-only connection")
	}
	tracker.HandleNetworkChange()
	if tracker.TaggedConnectionType(target) != "direct" {
		t.Fatalf("tagged type = %q, want %q", tracker.TaggedConnectionType(target), "direct")
	}
}

func TestConnectionTypeTrackerHandleNetworkChange(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tracker, _, cleanup := connectForConnectionTypeTest(t, ctx)
	defer cleanup()

	tracker.HandleNetworkChange()
}
