package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
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
	start.Long = `Starts the buyer-facing HTTP API and a libp2p QUIC host after logging in to the control pane.
Requires --control-url, --email, and --password. Register first: buyer register --control-url ... --name ... --email ... --password ...

Same as: knowledgeMesh mesh serve`

	root.AddCommand(start, buyer.NewRegisterCommand(), buyer.NewPromptCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
