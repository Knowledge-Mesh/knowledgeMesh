package types_test

import (
	"encoding/json"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestSellerNodeJSONRoundTrip(t *testing.T) {
	in := types.SellerNode{
		PeerID: "12D3KooWSeller",
		Skills: []types.Skill{
			{
				Name:       "summarization",
				ModelName:  "gpt-mini",
				ModelType:  "llm",
				TuningTier: "base",
				Price:      0.02,
			},
		},
		ModelName:  "gpt-mini",
		ModelType:  "llm",
		TuningTier: "base",
		Price:      0.02,
		Reputation: 4.8,
		OnDuty:     true,
		TokenLimits: types.RateLimits{
			MaxInputTokens:  4096,
			MaxOutputTokens: 1024,
			MaxTotalTokens:  5120,
		},
		Metadata: types.NodeMetadata{
			PeerID:     "12D3KooWSeller",
			Skills:     []types.Skill{{Name: "summarization", ModelName: "gpt-mini", ModelType: "llm", TuningTier: "base", Price: 0.02}},
			Reputation: 4.8,
			OnDuty:     true,
			ModelName:  "gpt-mini",
			ModelType:  "llm",
			TuningTier: "base",
			Price:      0.02,
			TokenLimits: types.RateLimits{
				MaxInputTokens:  4096,
				MaxOutputTokens: 1024,
				MaxTotalTokens:  5120,
			},
		},
		Usage: types.UsageCounters{
			RequestsServed: 12,
			InputTokens:    12000,
			OutputTokens:   6000,
			TotalTokens:    18000,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal seller node: %v", err)
	}

	var out types.SellerNode
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal seller node: %v", err)
	}

	if out.PeerID != in.PeerID {
		t.Fatalf("peer id mismatch: got %q want %q", out.PeerID, in.PeerID)
	}
	if len(out.Skills) != 1 {
		t.Fatalf("skills length mismatch: got %d want 1", len(out.Skills))
	}
	if out.Skills[0].ModelName != "gpt-mini" {
		t.Fatalf("model name mismatch: got %q", out.Skills[0].ModelName)
	}
	if out.Metadata.OnDuty != in.Metadata.OnDuty {
		t.Fatalf("metadata onDuty mismatch: got %v want %v", out.Metadata.OnDuty, in.Metadata.OnDuty)
	}
}

func TestInferenceRequestAndResponseJSONRoundTrip(t *testing.T) {
	reqIn := types.InferenceRequest{
		RequestID:   "req-123",
		BuyerPeerID: "12D3KooWBuyer",
		Skill: types.Skill{
			Name:       "qa",
			ModelName:  "mesh-qa-v1",
			ModelType:  "llm",
			TuningTier: "premium",
			Price:      0.05,
		},
		ModelName:  "mesh-qa-v1",
		ModelType:  "llm",
		TuningTier: "premium",
		Input:      "What is KnowledgeMesh?",
		TokenLimits: types.RateLimits{
			MaxInputTokens:  2048,
			MaxOutputTokens: 512,
			MaxTotalTokens:  2560,
		},
		MaxPrice: 0.15,
	}

	reqBytes, err := json.Marshal(reqIn)
	if err != nil {
		t.Fatalf("marshal inference request: %v", err)
	}

	var reqOut types.InferenceRequest
	if err := json.Unmarshal(reqBytes, &reqOut); err != nil {
		t.Fatalf("unmarshal inference request: %v", err)
	}

	if reqOut.RequestID != reqIn.RequestID {
		t.Fatalf("request id mismatch: got %q want %q", reqOut.RequestID, reqIn.RequestID)
	}
	if reqOut.TokenLimits.MaxTotalTokens != reqIn.TokenLimits.MaxTotalTokens {
		t.Fatalf("token limit mismatch: got %d want %d", reqOut.TokenLimits.MaxTotalTokens, reqIn.TokenLimits.MaxTotalTokens)
	}

	respIn := types.InferenceResponse{
		RequestID:    "req-123",
		SellerPeerID: "12D3KooWSeller",
		ModelName:    "mesh-qa-v1",
		ModelType:    "llm",
		TuningTier:   "premium",
		Output:       "KnowledgeMesh is a decentralized AI network.",
		PriceCharged: 0.12,
		Success:      true,
		TokenUsage: types.UsageCounters{
			RequestsServed: 1,
			InputTokens:    124,
			OutputTokens:   56,
			TotalTokens:    180,
		},
		SellerMetadata: types.NodeMetadata{
			PeerID:     "12D3KooWSeller",
			Skills:     []types.Skill{{Name: "qa", ModelName: "mesh-qa-v1", ModelType: "llm", TuningTier: "premium", Price: 0.05}},
			Reputation: 4.9,
			OnDuty:     true,
			ModelName:  "mesh-qa-v1",
			ModelType:  "llm",
			TuningTier: "premium",
			Price:      0.05,
			TokenLimits: types.RateLimits{
				MaxInputTokens:  2048,
				MaxOutputTokens: 512,
				MaxTotalTokens:  2560,
			},
		},
	}

	respBytes, err := json.Marshal(respIn)
	if err != nil {
		t.Fatalf("marshal inference response: %v", err)
	}

	var respOut types.InferenceResponse
	if err := json.Unmarshal(respBytes, &respOut); err != nil {
		t.Fatalf("unmarshal inference response: %v", err)
	}

	if !respOut.Success {
		t.Fatal("expected success=true after round-trip")
	}
	if respOut.TokenUsage.TotalTokens != respIn.TokenUsage.TotalTokens {
		t.Fatalf("total tokens mismatch: got %d want %d", respOut.TokenUsage.TotalTokens, respIn.TokenUsage.TotalTokens)
	}
	if respOut.SellerMetadata.ModelType != respIn.SellerMetadata.ModelType {
		t.Fatalf("seller metadata model type mismatch: got %q want %q", respOut.SellerMetadata.ModelType, respIn.SellerMetadata.ModelType)
	}
}
