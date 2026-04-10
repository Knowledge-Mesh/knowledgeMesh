package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
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

// NewHost builds a production-oriented libp2p host (QUIC+TCP, Noise, Yamux, NAT, relay, hole punch).
// Static relays: set LIBP2P_STATIC_RELAYS and/or use NewHostWithConfig with HostConfig.MergeStaticRelays.
func NewHost(ctx context.Context, listenAddr string) (host.Host, error) {
	h, _, err := NewHostWithConfig(ctx, DefaultHostConfig(listenAddr))
	return h, err
}

// TryConnectBootstrapPeers dials each bootstrap multiaddr (best-effort); logs failures without aborting.
// Used when DHT is enabled so AutoNAT has peers for dial-back probes.
func TryConnectBootstrapPeers(ctx context.Context, h host.Host, peers []string) {
	for _, addr := range peers {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		info, err := peer.AddrInfoFromString(addr)
		//err = autonat.NewAutoNATClient(h, nil, nil).DialBack(ctx, info.ID)
		//if err != nil {
		//	log.Printf("[p2p] bootstrap addr dial error %s: %v", addr, err)
		//}
		if err != nil {
			log.Printf("[p2p] bootstrap addr invalid %q: %v", addr, err)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, dialTimeout)
		err = h.Connect(cctx, *info)
		cancel()
		if err != nil {
			log.Printf("[p2p] bootstrap connect peer=%s: %v", info.ID, err)
			continue
		}
		log.Printf("[p2p] bootstrap connected peer=%s", info.ID)
	}
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
	resp, _, err := sendRequestWithRoute(ctx, h, target, pid, request)
	return resp, err
}

// CloseConnectionsToPeer closes all connections to a peer so a subsequent Connect redials fresh.
func CloseConnectionsToPeer(h host.Host, id peer.ID) {
	for _, c := range h.Network().ConnsToPeer(id) {
		_ = c.Close()
	}
}

func sendRequestWithRoute(ctx context.Context, h host.Host, target peer.ID, pid protocol.ID, request []byte) ([]byte, string, error) {
	s, err := h.NewStream(ctx, target, pid)
	if err != nil {
		GetP2PObserver().ObserveConnectionFailure()
		return nil, "", err
	}
	defer s.Close()
	route := connTypeDirect
	if isRelayAddr(s.Conn().RemoteMultiaddr()) {
		route = connTypeRelay
	}

	if _, err := s.Write(request); err != nil {
		GetP2PObserver().ObserveConnectionFailure()
		return nil, route, err
	}
	GetP2PObserver().ObserveTraffic(route, len(request))
	if err := s.CloseWrite(); err != nil {
		GetP2PObserver().ObserveConnectionFailure()
		return nil, route, err
	}
	resp, err := io.ReadAll(s)
	if err != nil {
		GetP2PObserver().ObserveConnectionFailure()
		return nil, route, err
	}
	if len(resp) == 0 {
		GetP2PObserver().ObserveConnectionFailure()
		return nil, route, ErrEmptyResponse
	}
	return resp, route, nil
}

func ConnectBootstrapPeers(ctx context.Context, h host.Host, peers []string) error {
	for _, addr := range peers {
		info, err := peer.AddrInfoFromString(addr)
		if err != nil {
			return fmt.Errorf("invalid bootstrap addr %q: %w", addr, err)
		}
		if err := h.Connect(ctx, *info); err != nil {
			GetP2PObserver().ObserveConnectionFailure()
			log.Printf("event=connect_bootstrap_failed peer=%s err=%v", info.ID.String(), err)
			return fmt.Errorf("connect bootstrap %s: %w", info.ID.String(), err)
		}
	}
	return nil
}

// ConnectToPeer adds transport addresses for id to the peerstore and dials. Addr strings must be
// libp2p multiaddrs without the trailing /p2p/<peerID> (same as host.Addrs()).
func ConnectToPeer(ctx context.Context, h host.Host, id peer.ID, transportAddrs []string) error {
	// Reuse an existing session when already connected instead of redialing per request.
	// This reduces handshake churn and transient drops on mobile networks.
	if h.Network().Connectedness(id) == network.Connected {
		return nil
	}

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
	if err := h.Connect(ctx, peer.AddrInfo{ID: id, Addrs: addrs}); err != nil {
		GetP2PObserver().ObserveConnectionFailure()
		log.Printf("event=connect_peer_failed peer=%s err=%v", id, err)
		return err
	}
	return nil
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
