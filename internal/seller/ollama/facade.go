package ollama

import (
	"context"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// SellerFacade wraps a Backend with optional config (for future real client base URL).
type SellerFacade struct {
	Backend Backend
	cfg     *types.OllamaSellerConfig
}

// NewSellerFacade uses MockBackend when b is nil.
func NewSellerFacade(cfg *types.OllamaSellerConfig, b Backend) *SellerFacade {
	if b == nil {
		b = NewMockBackend()
	}
	return &SellerFacade{Backend: b, cfg: cfg}
}

// ListModels lists models from the backend.
func (f *SellerFacade) ListModels(ctx context.Context) (ListModelsResponse, error) {
	return f.Backend.ListModels(ctx)
}

// Generate runs a single generate call (no stored context).
func (f *SellerFacade) Generate(ctx context.Context, in GenerateInput) (string, error) {
	return f.Backend.Generate(ctx, in)
}

// Chat runs a single chat call with only the provided messages (no history).
func (f *SellerFacade) Chat(ctx context.Context, in ChatInput) (string, error) {
	return f.Backend.Chat(ctx, in)
}
