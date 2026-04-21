package network

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

// Well-known environment variable for relay multiaddrs (comma-separated full multiaddrs with /p2p/<id>).
const EnvLibp2pStaticRelays = "LIBP2P_STATIC_RELAYS"

// EnvLibp2pBootstrapPeers lists bootstrap peer multiaddrs (comma-separated, each with /p2p/<id>) for optional DHT + outbound dials.
const EnvLibp2pBootstrapPeers = "LIBP2P_BOOTSTRAP_PEERS"

// EnvP2PDHT enables Kademlia DHT when set to 1, true, yes, or on (helps AutoNAT reachability probing when combined with bootstrap peers).
const EnvP2PDHT = "KM_P2P_DHT"

// Connection manager defaults (production-oriented).
const (
	connMgrLowWater    = 100
	connMgrHighWater   = 400
	connMgrGracePeriod = time.Minute

	// Safe transport tuning for mobile / unstable links:
	// - keepalive slightly tighter than yamux default (30s) to detect stale paths earlier.
	// - stream window above libp2p yamux default (16MiB) for medium-large payloads.
	// These values stay conservative to avoid aggressive memory/traffic behavior.
	yamuxKeepAliveInterval = 20 * time.Second
	yamuxMaxStreamWindow   = uint32(24 * 1024 * 1024) // 24MiB
	dialTimeout            = 25 * time.Second
)

// HostConfig groups libp2p host options so transports, relays, and listen addrs can evolve without
// changing call sites. Extend this struct when adding metrics, identity paths, or custom dialers.
type HostConfig struct {
	// ListenAddrs are libp2p listen multiaddrs. QUIC should be listed before TCP if both are used
	// so documentation matches “QUIC primary”; dial ranking is handled internally by libp2p.
	ListenAddrs []string
	// StaticRelayAddrs are full multiaddr strings (including /p2p/<peerID>) for circuit relay v2
	// servers used by AutoRelay. Critical for CGNAT/mobile when public inbound UDP/TCP is unavailable.
	StaticRelayAddrs []string
	// OnHolePunchService, if set, is invoked with the libp2p DCUtR service when the host is built.
	// Use this to wire HolePunchManager or other retry logic that calls DirectConnect.
	OnHolePunchService func(*holepunch.Service)
	// EnableP2PPrometheusExport registers km_p2p_* metrics with the default Prometheus registerer.
	// When nil, KM_P2P_PROMETHEUS_EXPORT controls export (default off).
	EnableP2PPrometheusExport *bool
	// EnableP2PDebug enables verbose P2P connectivity diagnostics.
	// When nil, KM_P2P_DEBUG controls it (default off).
	EnableP2PDebug *bool
	// EnableP2PDHT starts a Kademlia DHT (ModeAuto) so the node can discover peers and improve AutoNAT v2 reachability checks.
	// Use with P2PBootstrapPeers (well-known nodes or your relay as a bootstrap). Default off unless KM_P2P_DHT is set.
	EnableP2PDHT bool
	// P2PBootstrapPeers are full multiaddr strings (including /p2p/<peerID>) for outbound connects and DHT bootstrap.
	P2PBootstrapPeers []string
	// Identity, if set, is used as the libp2p host key (stable peer ID across restarts when persisted).
	// When nil, libp2p generates an ephemeral identity each run.
	Identity crypto.PrivKey
	// ServerMode, when true, skips ForceReachabilityPrivate so reachability follows AutoNAT.
	// Default false keeps ForceReachabilityPrivate enabled for NAT/mobile-friendly behavior.
	ServerMode bool
}

// DefaultHostConfig builds listen addresses (QUIC + TCP fallback) and loads static relays from
// built-in default public relays plus LIBP2P_STATIC_RELAYS, bootstrap peers from
// LIBP2P_BOOTSTRAP_PEERS, and optional DHT from KM_P2P_DHT when set.
func DefaultHostConfig(primaryListen string) HostConfig {
	if strings.TrimSpace(primaryListen) == "" {
		primaryListen = DefaultQUICListenAddr
	}
	cfg := HostConfig{
		ListenAddrs:       defaultListenAddrs(primaryListen),
		StaticRelayAddrs:  mergedStaticRelayAddrs(),
		P2PBootstrapPeers: bootstrapPeersFromEnv(),
		EnableP2PDHT:      parseP2PDHTFromEnv(),
	}
	return cfg
}

// MergeStaticRelays appends CLI-provided relay multiaddrs to cfg (after env defaults). Empty
// strings are skipped; parsing happens at NewHostWithConfig.
func (cfg *HostConfig) MergeStaticRelays(extra []string) {
	for _, s := range extra {
		s = strings.TrimSpace(s)
		if s != "" {
			cfg.StaticRelayAddrs = append(cfg.StaticRelayAddrs, s)
		}
	}
}

// MergeP2PBootstrapPeers appends CLI-provided bootstrap multiaddrs (after env defaults).
func (cfg *HostConfig) MergeP2PBootstrapPeers(extra []string) {
	for _, s := range extra {
		s = strings.TrimSpace(s)
		if s != "" {
			cfg.P2PBootstrapPeers = append(cfg.P2PBootstrapPeers, s)
		}
	}
}

func bootstrapPeersFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv(EnvLibp2pBootstrapPeers))
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseP2PDHTFromEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(EnvP2PDHT)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func defaultListenAddrs(primaryQUIC string) []string {
	// QUIC first (primary transport for modern NAT-friendly IETF QUIC).
	// TCP fallback: same host can accept inbound TCP dials when QUIC is blocked or for legacy peers.
	return []string{primaryQUIC, "/ip4/0.0.0.0/tcp/0"}
}

func staticRelaysFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv(EnvLibp2pStaticRelays))
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// StaticRelaysFromEnv returns relay multiaddrs configured via LIBP2P_STATIC_RELAYS.
// It returns a copy so callers can safely mutate the result.
func StaticRelaysFromEnv() []string {
	out := staticRelaysFromEnv()
	cp := make([]string, len(out))
	copy(cp, out)
	return cp
}

func mergedStaticRelayAddrs() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, s := range defaultStaticRelayAddrs() {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range staticRelaysFromEnv() {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ParseRelayAddrInfos converts full multiaddr strings into AddrInfo for AutoRelay.
func ParseRelayAddrInfos(multiaddrs []string) ([]peer.AddrInfo, error) {
	var out []peer.AddrInfo
	for _, s := range multiaddrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ai, err := peer.AddrInfoFromString(s)
		if err != nil {
			return nil, fmt.Errorf("multiaddr %q: %w", s, err)
		}
		out = append(out, *ai)
	}
	return out, nil
}

// NewHostWithConfig constructs a libp2p host with NAT traversal, relay, hole punching, and resource
// limits suitable for production on CGNAT/mobile networks.
// When cfg.EnableP2PDHT is true, the returned *dht.IpfsDHT is non-nil; the caller should Close it before or with the host.
func NewHostWithConfig(ctx context.Context, cfg HostConfig) (host.Host, *dht.IpfsDHT, error) {
	ApplyP2PMetricsExportForHost(cfg)
	ApplyP2PDebugForHost(cfg)

	if len(cfg.ListenAddrs) == 0 {
		cfg.ListenAddrs = defaultListenAddrs(DefaultQUICListenAddr)
	}

	var kad *dht.IpfsDHT
	opts, err := libp2pOptions(ctx, cfg, &kad)
	if err != nil {
		return nil, nil, err
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, nil, err
	}

	if cfg.EnableP2PDHT {
		if len(cfg.P2PBootstrapPeers) == 0 {
			log.Printf("[p2p] KM_P2P_DHT enabled but no bootstrap peers (set LIBP2P_BOOTSTRAP_PEERS or --p2p-bootstrap); AutoNAT may stay unknown longer")
		} else {
			TryConnectBootstrapPeers(ctx, h, cfg.P2PBootstrapPeers)
		}
		if kad != nil {
			if err := kad.Bootstrap(ctx); err != nil {
				log.Printf("[p2p] dht bootstrap: %v", err)
			}
		}
	}

	return h, kad, nil
}

// NewHostWithConfigAndHolePunch builds a host and a HolePunchManager wired to the libp2p DCUtR service.
// Call HolePunchManager.Start with the same context you use for graceful shutdown, then defer HolePunchManager.Close.
// When DHT is enabled, close the returned *dht.IpfsDHT before closing the host (see NewHostWithConfig).
func NewHostWithConfigAndHolePunch(ctx context.Context, cfg HostConfig) (host.Host, *HolePunchManager, *dht.IpfsDHT, error) {
	var hp *holepunch.Service
	cfg.OnHolePunchService = func(s *holepunch.Service) { hp = s }
	h, kad, err := NewHostWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	m := NewHolePunchManager(h, hp, DefaultHolePunchManagerConfig())
	return h, m, kad, nil
}

// holepunchExtra appends user hooks (e.g. capturing *holepunch.Service) to the DCUtR service options.
func holepunchExtra(cfg HostConfig) []holepunch.Option {
	if cfg.OnHolePunchService == nil {
		return nil
	}
	return []holepunch.Option{func(s *holepunch.Service) error {
		cfg.OnHolePunchService(s)
		return nil
	}}
}

func libp2pOptions(ctx context.Context, cfg HostConfig, kadPtr **dht.IpfsDHT) ([]libp2p.Option, error) {
	mgr, err := connmgr.NewConnManager(connMgrLowWater, connMgrHighWater, connmgr.WithGracePeriod(connMgrGracePeriod))
	if err != nil {
		return nil, fmt.Errorf("connmgr: %w", err)
	}

	relayInfos, err := ParseRelayAddrInfos(cfg.StaticRelayAddrs)
	if err != nil {
		return nil, err
	}

	bootstrapInfos, err := ParseRelayAddrInfos(cfg.P2PBootstrapPeers)
	if err != nil {
		return nil, fmt.Errorf("bootstrap peer: %w", err)
	}

	// Order: identity (optional) → transports (QUIC then TCP) → security → muxer → NAT → relay/autorelay → hole punch → connmgr → ping.
	var opts []libp2p.Option
	if cfg.Identity != nil {
		opts = append(opts, libp2p.Identity(cfg.Identity))
	}
	opts = append(opts,
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
		libp2p.WithDialTimeout(dialTimeout),

		// Explicit QUIC reuse keeps UDP socket management stable across many peer dials and
		// path changes; this helps connection continuity on mobile networks with IP churn.
		// We use libp2p's default constructor to preserve compatibility.
		libp2p.QUICReuse(quicreuse.NewConnManager),

		// Transports: QUIC preferred for performance and NAT; TCP fallback when QUIC is blocked.
		libp2p.Transport(quic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),

		// Noise: encrypted channels between peers (required for production libp2p security model).
		libp2p.Security(noise.ID, noise.New),

		// Tuned yamux settings complement QUIC by improving stream behavior for bigger payloads
		// while retaining default-safe behavior for small interactive messages.
		libp2p.Muxer(yamux.ID, tunedYamuxTransport()),

		// NATPortMap: UPnP/NAT-PMP port mapping on compatible routers so inbound QUIC/TCP can work.
		libp2p.NATPortMap(),

		// NATService: this node helps remote peers learn reachability via dial-back (good network citizen).
		libp2p.EnableNATService(),

		// AutoNAT v2: this node discovers whether it is publicly reachable (drives relay + addressing).
		libp2p.EnableAutoNATv2(),

		// Relay v2 client: can dial peers via circuit when direct paths fail (required for hole punching setup).
		libp2p.EnableRelay(),

		// AutoRelay: advertises relay addresses when behind NAT; static relays avoid discovery dependency.
		libp2p.EnableAutoRelayWithStaticRelays(relayInfos, autorelay.WithBootDelay(0),
			autorelay.WithMinCandidates(1),
			autorelay.WithNumRelays(4),
			autorelay.WithMaxCandidates(10),
			autorelay.WithBackoff(5*time.Second)),

		// DCUtR hole punching: coordinates direct connection upgrade over relay (see /libp2p/dcutr).
		libp2p.EnableHolePunching(holepunchExtra(cfg)...),

		libp2p.ConnectionManager(mgr),
		libp2p.Ping(true),
	)
	if !cfg.ServerMode {
		opts = append(opts, libp2p.ForceReachabilityPrivate())
	}

	if cfg.EnableP2PDHT {
		opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			dopts := []dht.Option{dht.Mode(dht.ModeAuto)}
			if len(bootstrapInfos) > 0 {
				dopts = append(dopts, dht.BootstrapPeers(bootstrapInfos...))
			}
			k, err := dht.New(ctx, h, dopts...)
			if err != nil {
				return nil, err
			}
			*kadPtr = k
			return k, nil
		}))
	}

	return opts, nil
}

func tunedYamuxTransport() *yamux.Transport {
	base := *yamux.DefaultTransport.Config()
	base.KeepAliveInterval = yamuxKeepAliveInterval
	base.MaxStreamWindowSize = yamuxMaxStreamWindow
	return (*yamux.Transport)(&base)
}
