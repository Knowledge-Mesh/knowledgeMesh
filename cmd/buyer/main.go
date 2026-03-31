package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "buyer",
		Short: "Buyer module CLI",
	}
	root.AddCommand(buyer.NewCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
