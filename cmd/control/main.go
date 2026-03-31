package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "control",
		Short: "Control module CLI",
	}
	root.AddCommand(control.NewCommand())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
