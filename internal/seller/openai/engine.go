package openai

import (
	"context"
	"errors"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// Engine calls OpenAI Chat Completions with the sandboxed prompt (implements seller.ModelEngine).
// Each call is stateless: a single user message, no conversation history.
type Engine struct {
	*Client
	cfg *types.OpenAISellerConfig
}

var ErrUnknownOpenAIModel = errors.New("model not declared in seller openai config")

func NewEngine(cfg *types.OpenAISellerConfig) *Engine {
	return &Engine{Client: NewClientFromConfig(cfg), cfg: cfg}
}

func (e *Engine) Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error) {
	model := resolveOpenAIModelID(e.cfg, strings.TrimSpace(req.ModelName))
	if model == "" {
		return "", ErrUnknownOpenAIModel
	}
	msgs := []chatMessagePart{{Role: "user", Content: prompt}}
	out, err := e.ChatCompletion(ctx, model, msgs)
	if err != nil {
		return "", err
	}
	return firstChoiceText(out), nil
}

func resolveOpenAIModelID(cfg *types.OpenAISellerConfig, requested string) string {
	if cfg == nil || len(cfg.Models) == 0 {
		if requested != "" {
			return requested
		}
		return "gpt-4o-mini"
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
