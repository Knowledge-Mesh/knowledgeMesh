package ollama

import (
	"context"
	"errors"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// Engine implements seller.ModelEngine using an Ollama-compatible Backend (mock or real).
type Engine struct {
	Backend Backend
	cfg     *types.OllamaSellerConfig
}

var ErrUnknownOllamaModel = errors.New("model not declared in seller ollama config")

// NewEngine uses MockBackend when b is nil.
func NewEngine(cfg *types.OllamaSellerConfig, b Backend) *Engine {
	if b == nil {
		b = NewMockBackend()
	}
	return &Engine{Backend: b, cfg: cfg}
}

func (e *Engine) Generate(ctx context.Context, prompt string, req types.InferenceRequest) (string, error) {
	model := resolveOllamaModelID(e.cfg, strings.TrimSpace(req.ModelName))
	if model == "" {
		return "", ErrUnknownOllamaModel
	}
	return e.Backend.Generate(ctx, GenerateInput{Model: model, Prompt: prompt})
}

func resolveOllamaModelID(cfg *types.OllamaSellerConfig, requested string) string {
	if cfg == nil || len(cfg.Models) == 0 {
		if requested != "" {
			return requested
		}
		return "llama3:mock"
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
