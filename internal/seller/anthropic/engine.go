package anthropic

import (
	"context"
	"errors"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// Engine calls Anthropic Messages API with the sandboxed prompt (implements seller.ModelEngine).
// No conversation history is sent; each call is a single user message.
type Engine struct {
	*Client
	cfg *types.AnthropicSellerConfig
}

var ErrUnknownAnthropicModel = errors.New("model not declared in seller anthropic config")

func NewEngine(cfg *types.AnthropicSellerConfig) *Engine {
	return &Engine{Client: NewClientFromConfig(cfg), cfg: cfg}
}

func (e *Engine) Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error) {
	model := resolveAnthropicModelID(e.cfg, strings.TrimSpace(req.ModelName))
	if model == "" {
		return "", ErrUnknownAnthropicModel
	}
	out, err := e.CreateMessage(ctx, model, prompt, 1024)
	if err != nil {
		return "", err
	}
	return extractText(out.Content), nil
}

func resolveAnthropicModelID(cfg *types.AnthropicSellerConfig, requested string) string {
	if cfg == nil || len(cfg.Models) == 0 {
		if requested != "" {
			return requested
		}
		return "claude-3-5-haiku-20241022"
	}
	for _, m := range cfg.Models {
		if m.ID == requested || m.Name == requested {
			if strings.TrimSpace(m.Name) != "" {
				return m.Name
			}
			return m.ID
		}
	}
	return ""
}
