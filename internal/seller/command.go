package seller

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start seller module",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("seller module started")
		},
	}
}
