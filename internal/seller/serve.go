package seller

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	"github.com/spf13/cobra"
)

// NewServeCommand runs a libp2p QUIC listener, registers inference, and executes via sandbox + mock engine.
func NewServeCommand() *cobra.Command {
	var (
		p2pAddr    string
		skillsRaw  string
		modelName  string
		modelType  string
		tuningTier string
		price      float64
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run libp2p QUIC seller node with sandboxed inference",
		Long: `Starts a QUIC listener, registers the inference protocol, and runs prompts through the sandbox before the model engine.

Copy a printed "dial this bootstrap" line for the buyer's --bootstrap flag. Set sellers-catalog peerId to this node's peer id.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			h, err := network.NewHost(ctx, p2pAddr)
			if err != nil {
				return err
			}
			defer h.Close()

			pid := h.ID().String()
			skills := parseSkills(skillsRaw, modelName, modelType, tuningTier, price)
			if len(skills) == 0 {
				skills = []types.Skill{{
					Name:       "chat",
					ModelName:  modelName,
					ModelType:  modelType,
					TuningTier: tuningTier,
					Price:      price,
				}}
			}
			node := types.SellerNode{
				PeerID:      pid,
				Skills:      skills,
				ModelName:   modelName,
				ModelType:   modelType,
				TuningTier:  tuningTier,
				Price:       price,
				OnDuty:      true,
				Reputation:  1,
			}
			runner := sandbox.NewRunner(sandbox.MockExecutor{}, 30*time.Second)
			inf := NewInferenceServiceForSeller(node, runner)
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
	cmd.Flags().StringVar(&skillsRaw, "skills", "chat", "Comma-separated skill names for matchmaking")
	cmd.Flags().StringVar(&modelName, "model-name", "kmg-mock-1", "Default model name on advertised skills")
	cmd.Flags().StringVar(&modelType, "model-type", "llm", "Model type")
	cmd.Flags().StringVar(&tuningTier, "tuning-tier", "base", "Tuning tier")
	cmd.Flags().Float64Var(&price, "price", 0, "Price used for matchmaking (must be within buyer max price if set)")
	return cmd
}
