package control

import "github.com/spf13/cobra"

// NewCommand is the control CLI root subcommand (currently `start`: libp2p + control protocol).
func NewCommand() *cobra.Command {
	return NewServeCommand()
}
