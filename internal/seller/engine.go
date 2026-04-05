package seller

import (
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/anthropic"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/openai"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// ModelEngineFromSellerNode returns OpenAI, Anthropic, or Ollama engine when on-duty and configured; otherwise mock.
// Precedence: OpenAI, Anthropic, Ollama.
// For Ollama: uses HTTPBackend (default base URL http://127.0.0.1:11434 when unset).
func ModelEngineFromSellerNode(node types.SellerNode) ModelEngine {
	if node.OnDuty && node.OpenAI != nil {
		return openai.NewEngine(node.OpenAI)
	}
	if node.OnDuty && node.Anthropic != nil {
		return anthropic.NewEngine(node.Anthropic)
	}
	if node.OnDuty && node.Ollama != nil {
		// HTTPBackend defaults empty/whitespace base URL to http://127.0.0.1:11434
		backend := ollama.NewHTTPBackend(node.Ollama.BaseURL)
		return ollama.NewEngine(node.Ollama, backend)
	}
	return MockModelEngine{}
}
