package mesh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/spf13/cobra"
)

// NewP2PDebugPeerCommand fetches peer connectivity diagnostics from the local debug API.
func NewP2PDebugPeerCommand() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "p2p-debug-peer <peerID>",
		Short: "Print peer connectivity debug information as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(base) == "" {
				base = strings.TrimSpace(os.Getenv(network.EnvP2PDebugHTTP))
			}
			base = strings.TrimRight(strings.TrimSpace(base), "/")
			if base == "" {
				return fmt.Errorf("set --http or %s (example: 127.0.0.1:9091)", network.EnvP2PDebugHTTP)
			}
			if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
				base = "http://" + base
			}
			url := base + "/debug/p2p/peer/" + strings.TrimPrefix(args[0], "/")
			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("debug endpoint %s: %s", resp.Status, strings.TrimSpace(string(body)))
			}
			var v map[string]any
			if err := json.Unmarshal(body, &v); err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(v)
		},
	}
	cmd.Flags().StringVar(&base, "http", "", "P2P debug HTTP base address (default from KM_P2P_DEBUG_HTTP)")
	return cmd
}
