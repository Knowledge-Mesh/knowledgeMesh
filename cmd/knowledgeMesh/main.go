package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/mesh"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "knowledgeMesh",
		Short: "KnowledgeMesh node CLI (buyer mesh + mock serve)",
	}

	meshCmd := &cobra.Command{
		Use:   "mesh",
		Short: "Buyer HTTP API with libp2p, matchmaking, and remote inference",
	}
	meshCmd.AddCommand(mesh.NewMeshServeCommand())
	meshCmd.AddCommand(mesh.NewP2PDebugPeerCommand())

	root.AddCommand(sandbox.NewServeCommand(), meshCmd)

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
