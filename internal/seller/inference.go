package seller

import (
	"context"
	"errors"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
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
	control      *control.Client
	sellerToken  string
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
// Pass non-nil control and sellerToken to report execution tracking to the control pane.
func NewInferenceServiceForSeller(node types.SellerNode, runner *sandbox.Runner, control *control.Client, sellerToken string) *InferenceService {
	svc := NewInferenceService(node.PeerID, runner, ModelEngineFromSellerNode(node))
	svc.control = control
	svc.sellerToken = sellerToken
	return svc
}

func (s *InferenceService) HandleInference(ctx context.Context, req types.InferenceRequest) (resp types.InferenceResponse, err error) {
	defer func() {
		if s.control != nil && s.sellerToken != "" {
			ok := err == nil && resp.Success
			tok := int64(resp.TokenUsage.TotalTokens)
			_ = s.control.PostSellerInferenceTracking(s.sellerToken, req.RequestID, tok, ok, nil)
		}
	}()

	if s.runner == nil {
		err = ErrInferenceFailed
		return types.InferenceResponse{}, err
	}

	sandboxedPrompt, err := s.runner.Run(ctx, req.Input)
	if err != nil {
		resp = types.InferenceResponse{
			RequestID:    req.RequestID,
			SellerPeerID: s.sellerPeerID,
			Status:       "error",
			ModelName:    req.ModelName,
			ModelType:    req.ModelType,
			TuningTier:   req.TuningTier,
			Success:      false,
			Error:        ErrInferenceFailed.Error(),
		}
		err = ErrInferenceFailed
		return resp, err
	}

	output, err := s.engine.Generate(ctx, sandboxedPrompt, req)
	if err != nil {
		resp = types.InferenceResponse{
			RequestID:    req.RequestID,
			SellerPeerID: s.sellerPeerID,
			Status:       "error",
			ModelName:    req.ModelName,
			ModelType:    req.ModelType,
			TuningTier:   req.TuningTier,
			Success:      false,
			Error:        ErrInferenceFailed.Error(),
		}
		err = ErrInferenceFailed
		return resp, err
	}

	inputTokens := estimateTokens(req.Input)
	outputTokens := estimateTokens(output)
	resp = types.InferenceResponse{
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
	}
	return resp, nil
}

func estimateTokens(v string) int {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	return len(strings.Fields(v))
}
