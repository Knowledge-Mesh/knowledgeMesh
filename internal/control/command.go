package control

import "github.com/spf13/cobra"

// NewCommand is the control CLI: `api` (HTTP + PostgreSQL) or `start` (libp2p control protocol).
func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "control",
		Short: "Control plane (HTTP API or libp2p node)",
	}
	root.AddCommand(NewAPICommand(), NewServeCommand())
	return root
}
