package control

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/protocol"
	"github.com/spf13/cobra"
	host "github.com/libp2p/go-libp2p/core/host"
)

// NewServeCommand runs a libp2p QUIC node with the control protocol registered.
func NewServeCommand() *cobra.Command {
	var p2pAddr string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start control plane libp2p node (QUIC + /knowledgemesh/control/1.0.0)",
		Long: `Listens for control streams on the knowledgeMesh control protocol. Send JSON such as
{"type":"ping"} and receive {"ok":true,"type":"pong",...}.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			h, err := network.NewHost(ctx, p2pAddr)
			if err != nil {
				return err
			}
			defer h.Close()

			RegisterHandler(h)

			log.Printf("control peer id: %s", h.ID().String())
			for _, a := range h.Addrs() {
				log.Printf("control listen: %s/p2p/%s", a, h.ID().String())
			}

			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&p2pAddr, "p2p-addr", network.DefaultQUICListenAddr, "libp2p QUIC listen multiaddr")
	return cmd
}

// RegisterHandler attaches the control protocol stream handler to the host.
func RegisterHandler(h host.Host) {
	network.RegisterRequestHandler(h, network.ProtocolControl, handleControl)
}

func handleControl(_ context.Context, reqBytes []byte) ([]byte, error) {
	var in map[string]any
	if len(reqBytes) > 0 {
		_ = json.Unmarshal(reqBytes, &in)
	}
	out := map[string]any{
		"ok":              true,
		"module":          "control",
		"protocolVersion": "1.0.0",
		"meshVersion":     protocol.Version,
	}
	if in != nil {
		if t, _ := in["type"].(string); t == "ping" {
			out["type"] = "pong"
		}
	}
	return json.Marshal(out)
}
