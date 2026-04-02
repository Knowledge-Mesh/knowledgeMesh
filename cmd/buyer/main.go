package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/mesh"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "buyer",
		Short: "Buyer module CLI",
	}

	start := mesh.NewMeshServeCommand()
	start.Use = "start"
	start.Short = "Start buyer HTTP API and libp2p node (matchmaking + remote inference)"
	start.Long = `Starts the buyer-facing HTTP API and a libp2p QUIC host, loads the sellers catalog,
connects to bootstrap peers, and routes chat/messages to matched sellers.

Same behavior as: knowledgeMesh mesh serve

Local two-terminal demo — terminal 1: go run ./cmd/seller serve --skills chat
Terminal 2: go run ./cmd/buyer start --demo --bootstrap '<seller-multiaddr>' --sellers-catalog examples/local-demo/sellers-catalog.json`

	root.AddCommand(start)

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
