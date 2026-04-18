package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "knowledgeMesh",
		Short: "KnowledgeMesh sandbox / mock API CLI",
		Long:  "Local sandbox HTTP API for development. Buyer mesh (serve, libp2p, control login) lives in the buyer binary: go run ./cmd/buyer serve",
	}

	root.AddCommand(sandbox.NewServeCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
