package relay

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	kmnet "github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	libp2p "github.com/libp2p/go-libp2p"
	lpnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
)

const (
	defaultListenAddr            = "/ip4/0.0.0.0/udp/4001/quic-v1"
	defaultConnLowWater          = 200
	defaultConnHighWater         = 800
	defaultConnGraceSeconds      = 60
	defaultMaxReservations       = 512
	defaultMaxCircuitsPerPeer    = 4
	defaultMaxBandwidthPerPeer   = 32 << 20 // 32 MiB per relay circuit window
	defaultRelayCircuitDurationS = 3600     // 1 hour
)

type Config struct {
	ListenAddr string

	ConnLowWater     int
	ConnHighWater    int
	ConnGracePeriod  time.Duration
	MaxReservations  int
	MaxCircuitsPeer  int
	MaxBandwidthPeer int64
	CircuitDuration  time.Duration
}

func defaultConfig() Config {
	return Config{
		ListenAddr:       defaultListenAddr,
		ConnLowWater:     defaultConnLowWater,
		ConnHighWater:    defaultConnHighWater,
		ConnGracePeriod:  time.Duration(defaultConnGraceSeconds) * time.Second,
		MaxReservations:  defaultMaxReservations,
		MaxCircuitsPeer:  defaultMaxCircuitsPerPeer,
		MaxBandwidthPeer: defaultMaxBandwidthPerPeer,
		CircuitDuration:  time.Duration(defaultRelayCircuitDurationS) * time.Second,
	}
}

func NewServeCommand() *cobra.Command {
	cfg := defaultConfig()
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run a minimal circuit relay v2 node",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(&cfg)
			return run(cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "libp2p listen multiaddr")
	cmd.Flags().IntVar(&cfg.MaxReservations, "max-reservations", cfg.MaxReservations, "Max relay reservations")
	cmd.Flags().IntVar(&cfg.MaxCircuitsPeer, "max-circuits-per-peer", cfg.MaxCircuitsPeer, "Max relayed circuits per peer")
	cmd.Flags().Int64Var(&cfg.MaxBandwidthPeer, "max-bandwidth-per-peer-bytes", cfg.MaxBandwidthPeer, "Max relayed bytes per peer circuit window")
	return cmd
}

func run(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	kmnet.SetPrometheusExport(kmnet.ParsePrometheusExportEnv())

	cm, err := connmgr.NewConnManager(cfg.ConnLowWater, cfg.ConnHighWater, connmgr.WithGracePeriod(cfg.ConnGracePeriod))
	if err != nil {
		return fmt.Errorf("conn manager: %w", err)
	}

	res := relayv2.DefaultResources()
	res.MaxReservations = cfg.MaxReservations
	res.MaxCircuits = cfg.MaxCircuitsPeer
	res.Limit = &relayv2.RelayLimit{
		Duration: cfg.CircuitDuration,
		Data:     cfg.MaxBandwidthPeer,
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr, "/ip4/0.0.0.0/tcp/4001"),
		libp2p.Transport(quic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.EnableRelay(),
		// Relay service mode: accepts relay reservations and relayed connections.
		libp2p.EnableRelayService(relayv2.WithResources(res), relayv2.WithMetricsTracer(&loggingRelayTracer{})),
		libp2p.ConnectionManager(cm),
		libp2p.Ping(true),
		libp2p.ForceReachabilityPublic(),
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return err
	}
	defer h.Close()

	h.Network().Notify(&connLogNotifee{})

	log.Printf("[relay] started peer_id=%s", h.ID())
	for _, a := range h.Addrs() {
		log.Printf("[relay] listen=%s/p2p/%s", a, h.ID())
	}
	log.Printf("[relay] limits max_reservations=%d max_circuits_per_peer=%d max_bandwidth_per_peer_bytes=%d", cfg.MaxReservations, cfg.MaxCircuitsPeer, cfg.MaxBandwidthPeer)

	<-ctx.Done()
	return nil
}

func applyEnv(cfg *Config) {
	cfg.ListenAddr = envString("RELAY_LISTEN_ADDR", cfg.ListenAddr)
	cfg.MaxReservations = envInt("RELAY_MAX_RESERVATIONS", cfg.MaxReservations)
	cfg.MaxCircuitsPeer = envInt("RELAY_MAX_CIRCUITS_PER_PEER", cfg.MaxCircuitsPeer)
	cfg.MaxBandwidthPeer = envInt64("RELAY_MAX_BANDWIDTH_PER_PEER_BYTES", cfg.MaxBandwidthPeer)
	cfg.ConnLowWater = envInt("RELAY_CONN_LOW_WATER", cfg.ConnLowWater)
	cfg.ConnHighWater = envInt("RELAY_CONN_HIGH_WATER", cfg.ConnHighWater)
	cfg.ConnGracePeriod = time.Duration(envInt("RELAY_CONN_GRACE_SECONDS", int(cfg.ConnGracePeriod/time.Second))) * time.Second
	cfg.CircuitDuration = time.Duration(envInt("RELAY_MAX_CIRCUIT_DURATION_SECONDS", int(cfg.CircuitDuration/time.Second))) * time.Second
}

func envString(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

type connLogNotifee struct{}

func (n *connLogNotifee) Connected(_ lpnet.Network, c lpnet.Conn) {
	log.Printf("[relay] connection opened peer=%s remote=%s", c.RemotePeer(), c.RemoteMultiaddr())
}
func (n *connLogNotifee) Disconnected(_ lpnet.Network, c lpnet.Conn) {
	log.Printf("[relay] connection closed peer=%s remote=%s", c.RemotePeer(), c.RemoteMultiaddr())
}
func (n *connLogNotifee) Listen(lpnet.Network, ma.Multiaddr)      {}
func (n *connLogNotifee) ListenClose(lpnet.Network, ma.Multiaddr) {}
