package seller

import (
	"context"
	"encoding/json"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	host "github.com/libp2p/go-libp2p/core/host"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// RegisterInferenceHandler serves inference over libp2p (JSON request/response, no streaming).
func RegisterInferenceHandler(h host.Host, inf *InferenceService) {
	network.RegisterRequestHandler(h, network.ProtocolInference, func(ctx context.Context, reqBytes []byte) ([]byte, error) {
		var req types.InferenceRequest
		if err := json.Unmarshal(reqBytes, &req); err != nil {
			b, _ := json.Marshal(types.InferenceResponse{Success: false, Error: "invalid request json"})
			return b, nil
		}
		resp, err := inf.HandleInference(ctx, req)
		if err != nil {
			b, _ := json.Marshal(resp)
			return b, nil
		}
		return json.Marshal(resp)
	})
}
