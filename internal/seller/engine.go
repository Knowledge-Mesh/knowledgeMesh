package seller

import (
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/anthropic"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/openai"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// ModelEngineFromSellerNode returns OpenAI, Anthropic, or Ollama engine when on-duty and configured; otherwise mock.
// Precedence: OpenAI, Anthropic, Ollama.
func ModelEngineFromSellerNode(node types.SellerNode) ModelEngine {
	if node.OnDuty && node.OpenAI != nil {
		return openai.NewEngine(node.OpenAI)
	}
	if node.OnDuty && node.Anthropic != nil {
		return anthropic.NewEngine(node.Anthropic)
	}
	if node.OnDuty && node.Ollama != nil {
		return ollama.NewEngine(node.Ollama, nil)
	}
	return MockModelEngine{}
}
