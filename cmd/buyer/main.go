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
		Short: "Buyer CLI: mesh HTTP API, libp2p, control login, registration",
	}

	serve := mesh.NewMeshServeCommand()
	serve.Use = "serve"
	serve.Aliases = []string{"start"}
	serve.Short = "Start buyer HTTP API and libp2p node (matchmaking + remote inference)"
	serve.Long = `Starts the buyer-facing HTTP API and a libp2p QUIC host after logging in to the control pane.
Requires --email and --password. --control-url is optional (defaults to http://127.0.0.1:8090; a warning is printed if omitted).
Register first: buyer register --name ... --email ... --password ...

Example:
  go run ./cmd/buyer serve --email you@example.com --password '...'`

	debugPeer := mesh.NewP2PDebugPeerCommand()

	root.AddCommand(serve, debugPeer, buyer.NewRegisterCommand(), buyer.NewPromptCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
