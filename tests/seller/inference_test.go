package seller_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

type failEngine struct{}

func (f failEngine) Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error) {
	return "", errors.New("boom")
}

func TestHandleInferenceSuccess(t *testing.T) {
	t.Parallel()

	runner := sandbox.NewRunner(sandbox.MockExecutor{Response: "sandboxed prompt"}, time.Second)
	svc := seller.NewInferenceService("seller-1", runner, seller.MockModelEngine{})
	req := types.InferenceRequest{
		RequestID:  "req-1",
		ModelName:  "mock-1",
		ModelType:  "llm",
		TuningTier: "base",
		Input:      "hello world",
	}

	resp, err := svc.HandleInference(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if !resp.Success || resp.Status != "success" {
		t.Fatalf("expected success status, got %+v", resp)
	}
	if resp.ModelName != "mock-1" || resp.ModelType != "llm" {
		t.Fatalf("model fields mismatch: %+v", resp)
	}
	if resp.TokenUsage.TotalTokens <= 0 {
		t.Fatalf("expected positive token usage, got %+v", resp.TokenUsage)
	}
}

func TestHandleInferenceSandboxFailureIsSanitized(t *testing.T) {
	t.Parallel()

	runner := sandbox.NewRunner(sandbox.MockExecutor{Err: errors.New("internal detail with prompt text")}, time.Second)
	svc := seller.NewInferenceService("seller-1", runner, seller.MockModelEngine{})
	req := types.InferenceRequest{
		RequestID: "req-2",
		Input:     "very-sensitive-prompt",
	}

	resp, err := svc.HandleInference(context.Background(), req)
	if !errors.Is(err, seller.ErrInferenceFailed) {
		t.Fatalf("expected ErrInferenceFailed, got %v", err)
	}
	if resp.Error != seller.ErrInferenceFailed.Error() {
		t.Fatalf("expected sanitized error, got %q", resp.Error)
	}
}

func TestHandleInferenceEngineFailure(t *testing.T) {
	t.Parallel()

	runner := sandbox.NewRunner(sandbox.MockExecutor{Response: "ok"}, time.Second)
	svc := seller.NewInferenceService("seller-1", runner, failEngine{})
	req := types.InferenceRequest{
		RequestID: "req-3",
		Input:     "hello",
	}

	resp, err := svc.HandleInference(context.Background(), req)
	if !errors.Is(err, seller.ErrInferenceFailed) {
		t.Fatalf("expected ErrInferenceFailed, got %v", err)
	}
	if resp.Status != "error" || resp.Success {
		t.Fatalf("expected error response, got %+v", resp)
	}
}
