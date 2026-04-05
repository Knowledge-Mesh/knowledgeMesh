package seller

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	"github.com/spf13/cobra"
)

// NewServeCommand runs a libp2p QUIC listener, registers inference, and executes via sandbox + mock engine.
// Requires control pane login (--control-url, --email, --password). Model list and duty come from PostgreSQL via the control API.
func NewServeCommand() *cobra.Command {
	var (
		p2pAddr    string
		controlURL string
		email      string
		password   string
		ollamaURL  string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run libp2p QUIC seller node (requires control pane login)",
		Long: "Starts a QUIC listener and registers the inference protocol. Authenticates to the control pane\n" +
			"and loads your declared models, duty flag, and rates from PostgreSQL.\n\n" +
			"Prerequisites: run `control api` with DATABASE_URL, register the seller (seller register or POST /v1/control/sellers/register),\n" +
			"and declare models (PUT /v1/control/sellers/me/models with a bearer token from login).\n\n" +
			"Copy the printed \"dial this bootstrap\" line for the buyer's --bootstrap flag; set sellers-catalog peerId to this node's peer id.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(controlURL) == "" || strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("required: --control-url, --email, and --password (seller must log in to the control pane)")
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			h, err := network.NewHost(ctx, p2pAddr)
			if err != nil {
				return err
			}
			defer h.Close()

			cc := control.NewClient(controlURL)
			tok, prof, err := cc.LoginSeller(email, password)
			if err != nil {
				return fmt.Errorf("control login: %w", err)
			}

			pid := h.ID().String()
			listenAddrs := make([]string, 0, len(h.Addrs()))
			for _, a := range h.Addrs() {
				listenAddrs = append(listenAddrs, a.String())
			}
			if _, err := cc.PostSellerPresence(tok, pid, listenAddrs); err != nil {
				log.Printf("warning: post presence to control: %v", err)
			}

			prof, err = cc.GetSellerMe(tok)
			if err != nil {
				return fmt.Errorf("control profile: %w", err)
			}

			var node types.SellerNode
			if strings.TrimSpace(ollamaURL) != "" {
				node = SellerNodeFromControlWithOllama(pid, prof, ollamaURL)
			} else {
				node = SellerNodeFromControl(pid, prof)
			}
			if len(node.Skills) == 0 {
				log.Printf("warning: no active models in control profile; declare models via PUT /v1/control/sellers/me/models")
			}
			log.Printf("seller loaded from control: onDuty=%v models=%d", node.OnDuty, len(node.Skills))

			runner := sandbox.NewRunner(sandbox.MockExecutor{}, 30*time.Second)
			inf := NewInferenceServiceForSeller(node, runner, cc, tok)
			RegisterInferenceHandler(h, inf)

			log.Printf("seller peer id: %s", pid)
			for _, a := range h.Addrs() {
				log.Printf("dial this bootstrap: %s/p2p/%s", a, pid)
			}

			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "libp2p QUIC listen multiaddr")
	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (required), e.g. http://127.0.0.1:8090")
	cmd.Flags().StringVar(&email, "email", "", "Seller email for control login (required)")
	cmd.Flags().StringVar(&password, "password", "", "Seller password for control login (required)")
	cmd.Flags().StringVar(&ollamaURL, "ollama-url", "", "Ollama server URL (e.g. http://127.0.0.1:11434). Enables real local inference instead of mock.")
	return cmd
}
