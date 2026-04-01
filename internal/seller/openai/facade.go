package openai

import (
	"context"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// SellerFacade exposes minimal OpenAI-compatible operations for the seller (no stored history).
type SellerFacade struct {
	*Client
}

func NewSellerFacade(cfg *types.OpenAISellerConfig) *SellerFacade {
	return &SellerFacade{Client: NewClientFromConfig(cfg)}
}

// ListModels returns models from OpenAI.
func (f *SellerFacade) ListModels(ctx context.Context) (ListModelsResponse, error) {
	return f.Client.ListModels(ctx)
}

// GenerateInput is a single-turn generate request.
type GenerateInput struct {
	Model     string
	Prompt    string
	MaxTokens int // reserved; OpenAI uses max_tokens in API — omitted for minimal compat
}

// Generate runs one chat completion with a single user message.
func (f *SellerFacade) Generate(ctx context.Context, in GenerateInput) (string, chatCompletionResponse, error) {
	msgs := []chatMessagePart{{Role: "user", Content: in.Prompt}}
	out, err := f.Client.ChatCompletion(ctx, in.Model, msgs)
	if err != nil {
		return "", chatCompletionResponse{}, err
	}
	text := firstChoiceText(out)
	return text, out, nil
}

// ChatInput is one system (optional) + one user message; no prior turns.
type ChatInput struct {
	Model     string
	System    string
	User      string
	MaxTokens int
}

// Chat sends only the messages for this request (no history).
func (f *SellerFacade) Chat(ctx context.Context, in ChatInput) (string, chatCompletionResponse, error) {
	var msgs []chatMessagePart
	if in.System != "" {
		msgs = append(msgs, chatMessagePart{Role: "system", Content: in.System})
	}
	msgs = append(msgs, chatMessagePart{Role: "user", Content: in.User})
	out, err := f.Client.ChatCompletion(ctx, in.Model, msgs)
	if err != nil {
		return "", chatCompletionResponse{}, err
	}
	return firstChoiceText(out), out, nil
}

func firstChoiceText(out chatCompletionResponse) string {
	if len(out.Choices) == 0 {
		return ""
	}
	return out.Choices[0].Message.Content
}
