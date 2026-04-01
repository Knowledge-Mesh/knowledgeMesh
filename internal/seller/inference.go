package seller

import (
	"context"
	"errors"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

var ErrInferenceFailed = errors.New("inference failed")

type ModelEngine interface {
	Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error)
}

type MockModelEngine struct{}

func (m MockModelEngine) Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error) {
	_ = ctx
	if strings.TrimSpace(prompt) == "" {
		return "", ErrInferenceFailed
	}
	return "mock-result:" + prompt, nil
}

type InferenceService struct {
	sellerPeerID string
	runner       *sandbox.Runner
	engine       ModelEngine
}

func NewInferenceService(sellerPeerID string, runner *sandbox.Runner, engine ModelEngine) *InferenceService {
	if engine == nil {
		engine = MockModelEngine{}
	}
	return &InferenceService{
		sellerPeerID: sellerPeerID,
		runner:       runner,
		engine:       engine,
	}
}

// NewInferenceServiceForSeller builds inference using sandbox + Anthropic engine when on-duty config is set.
func NewInferenceServiceForSeller(node types.SellerNode, runner *sandbox.Runner) *InferenceService {
	return NewInferenceService(node.PeerID, runner, ModelEngineFromSellerNode(node))
}

func (s *InferenceService) HandleInference(ctx context.Context, req types.InferenceRequest) (types.InferenceResponse, error) {
	if s.runner == nil {
		return types.InferenceResponse{}, ErrInferenceFailed
	}

	sandboxedPrompt, err := s.runner.Run(ctx, req.Input)
	if err != nil {
		return types.InferenceResponse{
			RequestID:    req.RequestID,
			SellerPeerID: s.sellerPeerID,
			Status:       "error",
			ModelName:    req.ModelName,
			ModelType:    req.ModelType,
			TuningTier:   req.TuningTier,
			Success:      false,
			Error:        ErrInferenceFailed.Error(),
		}, ErrInferenceFailed
	}

	output, err := s.engine.Generate(ctx, sandboxedPrompt, req)
	if err != nil {
		return types.InferenceResponse{
			RequestID:    req.RequestID,
			SellerPeerID: s.sellerPeerID,
			Status:       "error",
			ModelName:    req.ModelName,
			ModelType:    req.ModelType,
			TuningTier:   req.TuningTier,
			Success:      false,
			Error:        ErrInferenceFailed.Error(),
		}, ErrInferenceFailed
	}

	inputTokens := estimateTokens(req.Input)
	outputTokens := estimateTokens(output)
	return types.InferenceResponse{
		RequestID:    req.RequestID,
		SellerPeerID: s.sellerPeerID,
		Status:       "success",
		ModelName:    req.ModelName,
		ModelType:    req.ModelType,
		TuningTier:   req.TuningTier,
		Output:       output,
		Success:      true,
		TokenUsage: types.UsageCounters{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
	}, nil
}

func estimateTokens(v string) int {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	return len(strings.Fields(v))
}
