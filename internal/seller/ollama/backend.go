package ollama

import "context"

// Backend is the Ollama-compatible surface; swap MockBackend for an HTTP client to a real Ollama server.
type Backend interface {
	ListModels(ctx context.Context) (ListModelsResponse, error)
	Generate(ctx context.Context, in GenerateInput) (string, error)
	Chat(ctx context.Context, in ChatInput) (string, error)
}

// ListModelsResponse matches a minimal Ollama /api/tags style payload.
type ListModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo is a single model entry.
type ModelInfo struct {
	Name string `json:"name"`
}

// GenerateInput is a single /api/generate style request body (conceptual).
type GenerateInput struct {
	Model  string
	Prompt string
}

// ChatInput is a single /api/chat style request (no history beyond these messages).
type ChatInput struct {
	Model    string
	Messages []ChatMessage
}

// ChatMessage is one role/content pair.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
