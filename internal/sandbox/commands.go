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
	var apiAddr string
	var p2pAddr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start API and p2p node",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := network.NewHost(context.Background(), p2pAddr)
			if err != nil {
				return err
			}
			defer h.Close()

			server := api.NewServer(apiAddr)
			log.Printf("api listening on %s", apiAddr)
			log.Printf("p2p host started with id: %s", h.ID())
			return server.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&apiAddr, "api-addr", ":8080", "API listen address")
	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "p2p QUIC listen multiaddr")
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
