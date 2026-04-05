package seller_test

import (
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestSellerNodeFromControl_WithOllamaURL(t *testing.T) {
	prof := control.SellerProfile{
		SellerID: "s1",
		Name:     "ollama-seller",
		OnDuty:   true,
		Models: []control.SellerModelRecord{
			{ID: "m1", Name: "llama3.2:latest", SkillName: "chat", ModelName: "llama3.2:latest", ModelType: "llm", TuningTier: "base", Active: true},
		},
	}

	node := seller.SellerNodeFromControlWithOllama("peer1", prof, "http://127.0.0.1:11434")

	if node.Ollama == nil {
		t.Fatal("expected Ollama config to be set")
	}
	if node.Ollama.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("expected BaseURL http://127.0.0.1:11434, got %s", node.Ollama.BaseURL)
	}
	if len(node.Ollama.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(node.Ollama.Models))
	}
	if node.Ollama.Models[0].Name != "llama3.2:latest" {
		t.Fatalf("expected model llama3.2:latest, got %s", node.Ollama.Models[0].Name)
	}

	// Engine should use HTTPBackend
	engine := seller.ModelEngineFromSellerNode(node)
	ollamaEngine, ok := engine.(*ollama.Engine)
	if !ok {
		t.Fatalf("expected *ollama.Engine, got %T", engine)
	}
	_, isHTTP := ollamaEngine.Backend.(*ollama.HTTPBackend)
	if !isHTTP {
		t.Fatalf("expected *ollama.HTTPBackend, got %T", ollamaEngine.Backend)
	}
}

func TestSellerNodeFromControl_WithoutOllamaURL(t *testing.T) {
	prof := control.SellerProfile{
		SellerID: "s1",
		Name:     "ollama-seller",
		OnDuty:   true,
		Models: []control.SellerModelRecord{
			{ID: "m1", Name: "llama3.2:latest", SkillName: "chat", ModelName: "llama3.2:latest", ModelType: "llm", Active: true},
		},
	}

	node := seller.SellerNodeFromControlWithOllama("peer1", prof, "")

	if node.Ollama != nil {
		t.Fatal("expected Ollama config to be nil when no URL provided")
	}
}

func TestSellerNodeFromControl_OriginalUnchanged(t *testing.T) {
	prof := control.SellerProfile{
		SellerID: "s1",
		Name:     "api-seller",
		OnDuty:   true,
		Models: []control.SellerModelRecord{
			{ID: "m1", SkillName: "chat", ModelName: "claude-sonnet", Active: true, RatePerToken: 0.001},
		},
	}

	// Original function should still work without Ollama
	node := seller.SellerNodeFromControl("peer1", prof)
	if node.Ollama != nil {
		t.Fatal("original function should not set Ollama")
	}
	if !node.OnDuty {
		t.Fatal("expected on-duty")
	}
	if len(node.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(node.Skills))
	}

	_ = types.SellerNode{} // ensure types import used
}
