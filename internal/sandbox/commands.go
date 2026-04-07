package sandbox

import (
	"context"
	"fmt"
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/api"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/spf13/cobra"
)

func NewServeCommand() *cobra.Command {
	var (
		apiAddr string
		p2pAddr string
		relays  []string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start API and p2p node",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg := network.DefaultHostConfig(p2pAddr)
			cfg.MergeStaticRelays(relays)
			h, hpMgr, err := network.NewHostWithConfigAndHolePunch(ctx, cfg)
			if err != nil {
				return err
			}
			defer h.Close()
			defer hpMgr.Close()
			hpMgr.Start(ctx)
			connTracker := network.NewConnectionTypeTracker(h)
			defer connTracker.Close()
			connTracker.Start()

			netMon := network.NewNetworkMonitor(h, hpMgr, connTracker, network.DefaultNetworkMonitorConfig())
			netMon.Start(ctx)

			server := api.NewServer(apiAddr, nil)
			log.Printf("api listening on %s", apiAddr)
			log.Printf("p2p host started with id: %s", h.ID())
			return server.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&apiAddr, "api-addr", ":8080", "API listen address")
	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "p2p QUIC listen multiaddr")
	cmd.Flags().StringArrayVar(&relays, "relay", nil, "Circuit relay v2 multiaddr (repeatable); merged with LIBP2P_STATIC_RELAYS")
	return cmd
}

func NewDemoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run minimal demo workflow",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("demo workflow placeholder")
		},
	}
}
