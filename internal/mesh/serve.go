package mesh

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/api"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/spf13/cobra"
)

// NewMeshServeCommand starts the buyer API + libp2p host and loads seller catalog for matchmaking.
func NewMeshServeCommand() *cobra.Command {
	var (
		apiAddr      string
		p2pAddr      string
		bootstrap    []string
		relays       []string
		controlURL   string
		email        string
		password     string
		p2pDebug     bool
		p2pDebugHTTP string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start buyer HTTP API and libp2p node (requires control pane login)",
		Long: `Starts the buyer mesh API after authenticating to the control pane with your buyer email and password.

Prerequisites:
  • Run the control HTTP API with PostgreSQL (see: control api); register buyers and sellers there.
  • Sellers must be on-duty with models and presence (peer id) recorded in the control pane.
  • Pass --control-url, --email, and --password so this process can log in.

Example:
  go run ./cmd/knowledgeMesh mesh serve --control-url http://127.0.0.1:8090 --email you@example.com --password '...' \\
    --bootstrap '<seller-multiaddr>'

Use the printed session token as Authorization: Bearer or X-Session-ID for /v1/chat/completions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg := network.DefaultHostConfig(p2pAddr)
			cfg.MergeStaticRelays(relays)
			if p2pDebug || strings.TrimSpace(p2pDebugHTTP) != "" {
				v := true
				cfg.EnableP2PDebug = &v
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
			network.StartP2PDebugHTTPServerIfConfigured(ctx, p2pDebugHTTP, h, connTracker, hpMgr)

			netMon := network.NewNetworkMonitor(h, hpMgr, connTracker, network.DefaultNetworkMonitorConfig())
			netMon.Start(ctx)

			for _, boot := range bootstrap {
				if err := network.ConnectBootstrapPeers(ctx, h, []string{boot}); err != nil {
					return fmt.Errorf("bootstrap %s: %w", boot, err)
				}
			}

			if strings.TrimSpace(controlURL) == "" || strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("required: --control-url, --email, and --password (buyer must log in to the control pane)")
			}

			bm := buyer.NewManager()
			rt := NewRuntime(bm, h)
			rt.Router = network.NewMessageRouter(h, connTracker, hpMgr, network.DefaultMessageRouterConfig())
			rt.Control = control.NewClient(controlURL)
			st, err := rt.Login(email, password)
			if err != nil {
				return fmt.Errorf("control login: %w", err)
			}
			log.Printf("authenticated to control pane; for this session use Authorization: Bearer %s or X-Session-ID: %s", st.SessionID, st.SessionID)

			srv := api.NewServer(apiAddr, rt)
			log.Printf("api listening on %s", apiAddr)
			log.Printf("p2p host id: %s", h.ID().String())
			for _, a := range h.Addrs() {
				log.Printf("p2p listen: %s/p2p/%s", a, h.ID().String())
			}
			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&apiAddr, "api-addr", ":8080", "HTTP API listen address")
	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "libp2p QUIC listen multiaddr")
	cmd.Flags().StringArrayVar(&bootstrap, "bootstrap", nil, "Bootstrap peer multiaddr (repeatable), e.g. /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PeerID>")
	cmd.Flags().StringArrayVar(&relays, "relay", nil, "Circuit relay v2 multiaddr with /p2p/<relayID> (repeatable); merged with LIBP2P_STATIC_RELAYS for AutoRelay")
	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (required), e.g. http://127.0.0.1:8090")
	cmd.Flags().StringVar(&email, "email", "", "Buyer email for control pane login (required)")
	cmd.Flags().StringVar(&password, "password", "", "Buyer password for control pane login (required)")
	cmd.Flags().BoolVar(&p2pDebug, "p2p-debug", false, "Enable P2P connectivity diagnostics (or set KM_P2P_DEBUG=1)")
	cmd.Flags().StringVar(&p2pDebugHTTP, "p2p-debug-http", "", "Optional debug HTTP listen addr (example: 127.0.0.1:9091); implies --p2p-debug")
	return cmd
}
