package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "seller",
		Short: "Seller module CLI",
	}
	root.AddCommand(
		seller.NewControlRegisterCommand(),
		seller.NewControlSetupCommand(),
		seller.NewControlStatusCommand(),
		seller.NewControlDutyCommand(),
		seller.NewServeCommand(),
	)

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
