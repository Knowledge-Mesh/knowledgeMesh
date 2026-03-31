package buyer

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start buyer module",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("buyer module started")
		},
	}
}
