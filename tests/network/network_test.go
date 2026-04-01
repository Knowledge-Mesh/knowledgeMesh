package network_test

import (
	"context"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestRequestResponseOverProtocol(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h1, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatalf("host1 create: %v", err)
	}
	defer h1.Close()

	h2, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatalf("host2 create: %v", err)
	}
	defer h2.Close()

	network.RegisterRequestHandler(h2, network.ProtocolInference, func(ctx context.Context, req []byte) ([]byte, error) {
		return []byte("ok:" + string(req)), nil
	})

	addr := h2.Addrs()[0].Encapsulate(ma.StringCast("/p2p/" + h2.ID().String()))
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		t.Fatalf("addr info parse: %v", err)
	}
	if err := h1.Connect(ctx, *info); err != nil {
		t.Fatalf("connect: %v", err)
	}

	resp, err := network.SendRequest(ctx, h1, h2.ID(), network.ProtocolInference, []byte("ping"))
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	if string(resp) != "ok:ping" {
		t.Fatalf("response mismatch: got %q", string(resp))
	}
}

func TestLocalRegistryBootstrapList(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatalf("host create: %v", err)
	}
	defer h.Close()

	reg := network.NewLocalRegistry()
	reg.Register(h)

	list := reg.BootstrapList()
	if len(list) == 0 {
		t.Fatal("expected non-empty bootstrap list")
	}
}
