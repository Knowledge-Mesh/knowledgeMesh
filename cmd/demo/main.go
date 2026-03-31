package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "demo",
		Short: "Demo workflows CLI",
	}
	root.AddCommand(sandbox.NewDemoCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
