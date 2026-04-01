package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/api"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	"github.com/spf13/cobra"
)

// NewMeshServeCommand starts the buyer API + libp2p host and loads seller catalog for matchmaking.
func NewMeshServeCommand() *cobra.Command {
	var (
		apiAddr       string
		p2pAddr       string
		bootstrap     []string
		sellersPath   string
		demo          bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start buyer HTTP API and libp2p node (wired to matchmaking + seller inference)",
		Long: `Local two-terminal demo:

  Terminal 1 — seller (note the printed multiaddr and peer id):
    go run ./cmd/seller serve --skills chat

  Terminal 2 — buyer mesh API (use seller multiaddr as --bootstrap; point --sellers-catalog at a JSON array with matching peerId and skill "chat"):
    go run ./cmd/knowledgeMesh mesh serve --demo --bootstrap '<seller-multiaddr>' --sellers-catalog examples/local-demo/sellers-catalog.json

  Then POST /api/v1/buyer/login or use --demo printed X-Session-ID, and call /v1/chat/completions with that session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			h, err := network.NewHost(ctx, p2pAddr)
			if err != nil {
				return err
			}
			defer h.Close()

			for _, boot := range bootstrap {
				if err := network.ConnectBootstrapPeers(ctx, h, []string{boot}); err != nil {
					return fmt.Errorf("bootstrap %s: %w", boot, err)
				}
			}

			bm := buyer.NewManager()
			var demoSession string
			if demo {
				if _, err := bm.Register("demo@local", "demo", "demo"); err != nil && err != buyer.ErrBuyerExists {
					return err
				}
				st, err := bm.Login("demo", "demo")
				if err != nil {
					return err
				}
				demoSession = st.SessionID
				log.Printf("demo mode: use HTTP header X-Session-ID: %s", demoSession)
			}

			rt := NewRuntime(bm, h)
			if sellersPath != "" {
				b, err := os.ReadFile(sellersPath)
				if err != nil {
					return err
				}
				var nodes []types.SellerNode
				if err := json.Unmarshal(b, &nodes); err != nil {
					return err
				}
				rt.SetSellers(nodes)
			}

			srv := api.NewServer(apiAddr, rt)
			log.Printf("api listening on %s", apiAddr)
			log.Printf("p2p host id: %s", h.ID().String())
			for _, a := range h.Addrs() {
				log.Printf("p2p listen: %s/p2p/%s", a, h.ID().String())
			}
			_ = demoSession
			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&apiAddr, "api-addr", ":8080", "HTTP API listen address")
	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "libp2p QUIC listen multiaddr")
	cmd.Flags().StringArrayVar(&bootstrap, "bootstrap", nil, "Bootstrap peer multiaddr (repeatable), e.g. /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PeerID>")
	cmd.Flags().StringVar(&sellersPath, "sellers-catalog", "", "JSON array of SellerNode for matchmaking")
	cmd.Flags().BoolVar(&demo, "demo", false, "Register/login a demo buyer and print session id")
	return cmd
}
