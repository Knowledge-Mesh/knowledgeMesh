package network_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func connectHostsForMessageRouterTest(t *testing.T, ctx context.Context) (*network.ConnectionTypeTracker, *network.MessageRouter, peer.ID, func()) {
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

	network.RegisterRequestHandler(h2, network.ProtocolInference, func(_ context.Context, req []byte) ([]byte, error) {
		return []byte("ok:" + string(req)), nil
	})

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
	router := network.NewMessageRouter(h1, tracker, nil, network.DefaultMessageRouterConfig())

	cleanup := func() {
		tracker.Close()
		_ = h2.Close()
		_ = h1.Close()
	}
	return tracker, router, h2.ID(), cleanup
}

func TestMessageRouterSmallMessageAnyPath(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, router, target, cleanup := connectHostsForMessageRouterTest(t, ctx)
	defer cleanup()

	resp, err := router.SendRequest(ctx, target, network.ProtocolInference, []byte("ping"))
	if err != nil {
		t.Fatalf("router send small: %v", err)
	}
	if string(resp) != "ok:ping" {
		t.Fatalf("response mismatch: %q", string(resp))
	}

	m := router.Metrics()
	if m.TotalSent.Load() != 1 {
		t.Fatalf("total sent = %d, want 1", m.TotalSent.Load())
	}
	if m.DirectSent.Load()+m.RelaySent.Load() != 1 {
		t.Fatalf("route counters mismatch: direct=%d relay=%d", m.DirectSent.Load(), m.RelaySent.Load())
	}
}

func TestMessageRouterLargeMessagePrefersDirect(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	tracker, router, target, cleanup := connectHostsForMessageRouterTest(t, ctx)
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for !tracker.IsDirectConnection(target) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}

	large := []byte(strings.Repeat("x", 40*1024))
	resp, err := router.SendRequest(ctx, target, network.ProtocolInference, large)
	if err != nil {
		t.Fatalf("router send large: %v", err)
	}
	if len(resp) == 0 {
		t.Fatal("expected non-empty response")
	}

	m := router.Metrics()
	if m.TotalSent.Load() != 1 {
		t.Fatalf("total sent = %d, want 1", m.TotalSent.Load())
	}
	if m.DirectSent.Load() == 0 {
		t.Fatalf("expected direct route for large message; direct=%d relay=%d", m.DirectSent.Load(), m.RelaySent.Load())
	}
}
