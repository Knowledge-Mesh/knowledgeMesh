package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	ma "github.com/multiformats/go-multiaddr"
)

const DefaultQUICListenAddr = "/ip4/0.0.0.0/udp/0/quic-v1"

const (
	ProtocolControl   protocol.ID = "/knowledgemesh/control/1.0.0"
	ProtocolInference protocol.ID = "/knowledgemesh/inference/1.0.0"
)

var ErrEmptyResponse = errors.New("empty response from peer")

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type RequestHandler func(context.Context, []byte) ([]byte, error)

func NewHost(_ context.Context, listenAddr string) (host.Host, error) {
	return libp2p.New(
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Transport(quic.NewTransport),
	)
}

func RegisterRequestHandler(h host.Host, pid protocol.ID, handler RequestHandler) {
	h.SetStreamHandler(pid, func(s network.Stream) {
		defer s.Close()

		reqBytes, err := io.ReadAll(s)
		if err != nil {
			_, _ = s.Write([]byte(`{"error":"read request failed"}`))
			return
		}
		respBytes, err := handler(context.Background(), reqBytes)
		if err != nil {
			_, _ = s.Write([]byte(`{"error":"handler failed"}`))
			return
		}
		if len(respBytes) == 0 {
			_, _ = s.Write([]byte(`{"error":"empty response"}`))
			return
		}
		_, _ = s.Write(respBytes)
	})
}

func SendRequest(ctx context.Context, h host.Host, target peer.ID, pid protocol.ID, request []byte) ([]byte, error) {
	s, err := h.NewStream(ctx, target, pid)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	if _, err := s.Write(request); err != nil {
		return nil, err
	}
	if err := s.CloseWrite(); err != nil {
		return nil, err
	}
	resp, err := io.ReadAll(s)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, ErrEmptyResponse
	}
	return resp, nil
}

func ConnectBootstrapPeers(ctx context.Context, h host.Host, peers []string) error {
	for _, addr := range peers {
		info, err := peer.AddrInfoFromString(addr)
		if err != nil {
			return fmt.Errorf("invalid bootstrap addr %q: %w", addr, err)
		}
		if err := h.Connect(ctx, *info); err != nil {
			return fmt.Errorf("connect bootstrap %s: %w", info.ID.String(), err)
		}
	}
	return nil
}

// ConnectToPeer adds transport addresses for id to the peerstore and dials. Addr strings must be
// libp2p multiaddrs without the trailing /p2p/<peerID> (same as host.Addrs()).
func ConnectToPeer(ctx context.Context, h host.Host, id peer.ID, transportAddrs []string) error {
	var addrs []ma.Multiaddr
	for _, s := range transportAddrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		m, err := ma.NewMultiaddr(s)
		if err != nil {
			return fmt.Errorf("invalid multiaddr %q: %w", s, err)
		}
		addrs = append(addrs, m)
	}
	if len(addrs) == 0 {
		return nil
	}
	return h.Connect(ctx, peer.AddrInfo{ID: id, Addrs: addrs})
}

type LocalRegistry struct {
	mu    sync.RWMutex
	peers map[peer.ID][]ma.Multiaddr
}

func NewLocalRegistry() *LocalRegistry {
	return &LocalRegistry{
		peers: map[peer.ID][]ma.Multiaddr{},
	}
}

func (r *LocalRegistry) Register(h host.Host) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[h.ID()] = h.Addrs()
}

func (r *LocalRegistry) BootstrapList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.peers))
	for id, addrs := range r.peers {
		for _, a := range addrs {
			out = append(out, a.Encapsulate(ma.StringCast("/p2p/"+id.String())).String())
		}
	}
	return out
}
