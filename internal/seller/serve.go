package seller

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	host "github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
)

// NewServeCommand runs a libp2p QUIC listener, registers inference, and executes via sandbox + configured model backends (Ollama/OpenAI/Anthropic from control API).
// Requires control pane login (--email, --password; --control-url optional, default http://127.0.0.1:8090). Model list and duty come from PostgreSQL via the control API.
func NewServeCommand() *cobra.Command {
	var (
		p2pAddr     string
		relays      []string
		bootstrap   []string
		p2pDHT      bool
		controlURL  string
		email       string
		password    string
		p2pIdentity string
		serverMode  bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run libp2p QUIC seller node (requires control pane login)",
		Long: "Starts a QUIC listener and registers the inference protocol. Authenticates to the control pane\n" +
			"and loads your declared models, duty flag, and rates from PostgreSQL.\n\n" +
			"Prerequisites: run `control api` with DATABASE_URL, register the seller (seller register or POST /v1/control/sellers/register),\n" +
			"and declare models (PUT /v1/control/sellers/me/models with a bearer token from login).\n\n" +
			"Copy the printed \"dial this bootstrap\" line for the buyer's --bootstrap flag; set sellers-catalog peerId to this node's peer id.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("required: --email and --password (seller must log in to the control pane)")
			}

			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cc := control.NewClient(controlURL)
			tok, prof, err := cc.LoginSeller(email, password)
			if err != nil {
				return fmt.Errorf("control login: %w", err)
			}
			if strings.TrimSpace(prof.SellerID) == "" {
				return fmt.Errorf("control login: empty seller id")
			}

			priv, idPath, err := network.LoadOrCreateAccountP2PIdentity(network.AccountRoleSeller, controlURL, prof.SellerID, p2pIdentity)
			if err != nil {
				return fmt.Errorf("p2p identity: %w", err)
			}
			log.Printf("[seller] libp2p identity: %s", idPath)

			cfg := network.DefaultHostConfig(p2pAddr)
			cfg.Identity = priv
			cfg.ServerMode = serverMode
			cfg.MergeStaticRelays(relays)
			cfg.MergeP2PBootstrapPeers(bootstrap)
			if p2pDHT {
				cfg.EnableP2PDHT = true
			}
			h, hpMgr, kad, err := network.NewHostWithConfigAndHolePunch(ctx, cfg)
			if err != nil {
				return err
			}
			defer h.Close()
			defer hpMgr.Close()
			defer func() {
				if kad != nil {
					_ = kad.Close()
				}
			}()
			hpMgr.Start(ctx)
			connTracker := network.NewConnectionTypeTracker(h)
			defer connTracker.Close()
			connTracker.Start()
			network.StartP2PDebugMonitors(ctx, h)
			network.StartP2PDebugHTTPServerIfConfigured(ctx, "", h, connTracker, hpMgr)

			netMon := network.NewNetworkMonitor(h, hpMgr, connTracker, network.DefaultNetworkMonitorConfig())
			netMon.Start(ctx)
			network.StartSellerReachabilityLogger(ctx, h)

			netMon.OnAutoNATRefresh = func(ev network.NetworkChangeEvent) {
				log.Printf("network changed (ip=%v iface=%v): libp2p AutoNAT continues probing in background", ev.IPChanged, ev.InterfaceChanged)
			}
			netMon.OnReAdvertise = func(_ network.NetworkChangeEvent) {
				listenAddrs := make([]string, 0, len(h.Addrs()))
				for _, a := range h.Addrs() {
					listenAddrs = append(listenAddrs, a.String())
				}
				if _, err := cc.PostSellerPresence(tok, h.ID().String(), listenAddrs); err != nil {
					log.Printf("warning: re-advertise seller presence after network change: %v", err)
				}
			}

			pid := h.ID().String()
			waitForRelayAddrs(ctx, h, 15*time.Second)
			listenAddrs := make([]string, 0, len(h.Addrs()))
			for _, a := range h.Addrs() {
				listenAddrs = append(listenAddrs, a.String())
			}
			if _, err := cc.PostSellerPresence(tok, pid, listenAddrs); err != nil {
				log.Printf("warning: post presence to control: %v", err)
			}

			prof, err = cc.GetSellerMe(tok)
			if err != nil {
				return fmt.Errorf("control profile: %w", err)
			}

			node := SellerNodeFromControl(pid, prof)
			if len(node.Skills) == 0 {
				log.Printf("warning: no active models in control profile; declare models via PUT /v1/control/sellers/me/models")
			}
			log.Printf("seller loaded from control: onDuty=%v models=%d ollamaConfigured=%v", node.OnDuty, len(node.Skills), prof.Ollama != nil)

			runner := sandbox.NewRunner(sandbox.PassthroughExecutor{}, 30*time.Second)
			inf := NewInferenceServiceForSeller(node, runner, cc, tok)
			RegisterInferenceHandler(h, inf)

			log.Printf("seller peer id: %s", pid)
			for _, a := range h.Addrs() {
				log.Printf("dial this bootstrap: %s/p2p/%s", a, pid)
			}

			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "libp2p QUIC listen multiaddr (TCP /ip4/0.0.0.0/tcp/0 is added automatically)")
	cmd.Flags().StringArrayVar(&relays, "relay", nil, "Circuit relay v2 multiaddr with /p2p/<relayID> (repeatable); merged with LIBP2P_STATIC_RELAYS")
	cmd.Flags().StringArrayVar(&bootstrap, "p2p-bootstrap", nil, "Bootstrap peer multiaddr with /p2p/<peerID> (repeatable); merged with LIBP2P_BOOTSTRAP_PEERS; use with --p2p-dht for AutoNAT reachability")
	cmd.Flags().BoolVar(&p2pDHT, "p2p-dht", false, "Enable Kademlia DHT (ModeAuto) for peer discovery and AutoNAT v2 probes; also set KM_P2P_DHT=1 or bootstrap peers via env")
	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&email, "email", "", "Seller email for control login (required)")
	cmd.Flags().StringVar(&password, "password", "", "Seller password for control login (required)")
	cmd.Flags().StringVar(&p2pIdentity, "p2p-identity", "", "Path to persisted libp2p identity key (optional; default: per-account file under user config, or "+network.EnvP2PIdentityFile+")")
	cmd.Flags().BoolVar(&serverMode, "server-mode", false, "If set, omit ForceReachabilityPrivate (let AutoNAT decide; use on public servers with fixed ports)")
	return cmd
}

// waitForRelayAddrs polls h.Addrs() until at least one /p2p-circuit address
// appears, or the timeout elapses. This ensures the seller posts reachable relay
// addresses to the control plane instead of only local/LAN addresses.
func waitForRelayAddrs(ctx context.Context, h host.Host, timeout time.Duration) {
	deadline := time.After(timeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, a := range h.Addrs() {
			if _, err := a.ValueForProtocol(ma.P_CIRCUIT); err == nil {
				log.Printf("[seller] relay address ready: %s", a)
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			log.Printf("warning: no relay addresses after %s, posting local-only presence", timeout)
			return
		case <-tick.C:
		}
	}
}
