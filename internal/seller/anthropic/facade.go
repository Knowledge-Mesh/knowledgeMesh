package anthropic

import (
	"context"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// SellerFacade exposes minimal Anthropic-compatible operations for the seller (no history).
type SellerFacade struct {
	*Client
}

func NewSellerFacade(cfg *types.AnthropicSellerConfig) *SellerFacade {
	return &SellerFacade{Client: NewClientFromConfig(cfg)}
}

// ListModels returns remote models from Anthropic (or error if API unavailable).
func (f *SellerFacade) ListModels(ctx context.Context) (ListModelsResponse, error) {
	return f.Client.ListModels(ctx)
}

// GenerateInput is a minimal generate request (single prompt, no chat history).
type GenerateInput struct {
	Model     string
	Prompt    string
	MaxTokens int
}

// Generate runs one stateless completion via Messages API.
func (f *SellerFacade) Generate(ctx context.Context, in GenerateInput) (string, messagesUsage, error) {
	out, err := f.Client.CreateMessage(ctx, in.Model, in.Prompt, in.MaxTokens)
	if err != nil {
		return "", messagesUsage{}, err
	}
	text := extractText(out.Content)
	return text, out.Usage, nil
}

// ChatInput is a minimal chat request (optional system, single user message; no history).
type ChatInput struct {
	Model     string
	System    string
	User      string
	MaxTokens int
}

// Chat sends one user message with optional system prompt; no prior turns.
func (f *SellerFacade) Chat(ctx context.Context, in ChatInput) (string, messagesUsage, error) {
	userText := in.User
	if strings.TrimSpace(in.System) != "" {
		userText = "(system: " + in.System + ")\n\n" + in.User
	}
	out, err := f.Client.CreateMessage(ctx, in.Model, userText, in.MaxTokens)
	if err != nil {
		return "", messagesUsage{}, err
	}
	text := extractText(out.Content)
	return text, out.Usage, nil
}

func extractText(blocks []contentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}
