package seller

import (
	"fmt"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/spf13/cobra"
)

// NewControlRegisterCommand registers a seller on the control pane (PostgreSQL).
func NewControlRegisterCommand() *cobra.Command {
	var (
		controlURL string
		name       string
		email      string
		password   string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a seller account on the control pane",
		Long:  `Requires a running control API (control api) and DATABASE_URL. Stores credentials in PostgreSQL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)
			if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" || password == "" {
				return fmt.Errorf("required: --name, --email, and --password")
			}
			cc := control.NewClient(controlURL)
			id, err := cc.RegisterSeller(name, email, password)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "registered seller id: %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&email, "email", "", "Email")
	cmd.Flags().StringVar(&password, "password", "", "Password")
	return cmd
}
