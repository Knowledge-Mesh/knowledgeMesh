package control

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start control module",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("control module started")
		},
	}
}
