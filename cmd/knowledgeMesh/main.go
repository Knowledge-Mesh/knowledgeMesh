package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "knowledgeMesh",
		Short: "KnowledgeMesh node CLI",
	}

	root.AddCommand(sandbox.NewServeCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
