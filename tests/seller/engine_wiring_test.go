package seller_test

import (
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestModelEngineFromSellerNode_OllamaWithBaseURL(t *testing.T) {
	node := types.SellerNode{
		OnDuty: true,
		Ollama: &types.OllamaSellerConfig{
			BaseURL: "http://127.0.0.1:11434",
			Models: []types.OllamaModelDecl{
				{ID: "llama3.2", Name: "llama3.2:latest"},
			},
		},
	}

	engine := seller.ModelEngineFromSellerNode(node)

	// Should be an Ollama engine, not mock
	ollamaEngine, ok := engine.(*ollama.Engine)
	if !ok {
		t.Fatalf("expected *ollama.Engine, got %T", engine)
	}

	// Backend should be HTTPBackend, not MockBackend
	_, isHTTP := ollamaEngine.Backend.(*ollama.HTTPBackend)
	if !isHTTP {
		t.Fatalf("expected *ollama.HTTPBackend, got %T", ollamaEngine.Backend)
	}
}

func TestModelEngineFromSellerNode_OllamaWithoutBaseURL(t *testing.T) {
	node := types.SellerNode{
		OnDuty: true,
		Ollama: &types.OllamaSellerConfig{
			Models: []types.OllamaModelDecl{
				{ID: "llama3.2", Name: "llama3.2:latest"},
			},
		},
	}

	engine := seller.ModelEngineFromSellerNode(node)

	ollamaEngine, ok := engine.(*ollama.Engine)
	if !ok {
		t.Fatalf("expected *ollama.Engine, got %T", engine)
	}

	// Empty BaseURL uses HTTPBackend defaulting to http://127.0.0.1:11434
	_, isHTTP := ollamaEngine.Backend.(*ollama.HTTPBackend)
	if !isHTTP {
		t.Fatalf("expected *ollama.HTTPBackend, got %T", ollamaEngine.Backend)
	}
}

func TestModelEngineFromSellerNode_OllamaWhitespaceBaseURLUsesHTTP(t *testing.T) {
	node := types.SellerNode{
		OnDuty: true,
		Ollama: &types.OllamaSellerConfig{
			BaseURL: "  ",
			Models: []types.OllamaModelDecl{
				{ID: "llama3.2", Name: "llama3.2:latest"},
			},
		},
	}

	engine := seller.ModelEngineFromSellerNode(node)

	ollamaEngine, ok := engine.(*ollama.Engine)
	if !ok {
		t.Fatalf("expected *ollama.Engine, got %T", engine)
	}

	// Whitespace-only BaseURL is treated as unset and defaults to 127.0.0.1:11434
	_, isHTTP := ollamaEngine.Backend.(*ollama.HTTPBackend)
	if !isHTTP {
		t.Fatalf("expected *ollama.HTTPBackend for whitespace BaseURL, got %T", ollamaEngine.Backend)
	}
}

func TestModelEngineFromSellerNode_OffDutyReturnsMock(t *testing.T) {
	node := types.SellerNode{
		OnDuty: false,
		Ollama: &types.OllamaSellerConfig{
			BaseURL: "http://127.0.0.1:11434",
		},
	}

	engine := seller.ModelEngineFromSellerNode(node)

	_, isMock := engine.(seller.MockModelEngine)
	if !isMock {
		t.Fatalf("expected MockModelEngine for off-duty node, got %T", engine)
	}
}
