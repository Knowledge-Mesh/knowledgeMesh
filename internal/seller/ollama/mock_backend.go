package ollama

import (
	"context"
	"fmt"
	"strings"
)

// MockBackend is a stand-in until a real Ollama HTTP backend is wired in.
type MockBackend struct{}

func NewMockBackend() *MockBackend {
	return &MockBackend{}
}

func (m *MockBackend) ListModels(_ context.Context) (ListModelsResponse, error) {
	return ListModelsResponse{
		Models: []ModelInfo{
			{Name: "llama3:mock"},
			{Name: "phi3:mock"},
		},
	}, nil
}

func (m *MockBackend) Generate(_ context.Context, in GenerateInput) (string, error) {
	return fmt.Sprintf("mock-ollama-generate:%s:%s", in.Model, in.Prompt), nil
}

func (m *MockBackend) Chat(_ context.Context, in ChatInput) (string, error) {
	var parts []string
	for _, msg := range in.Messages {
		parts = append(parts, msg.Role+":"+msg.Content)
	}
	return "mock-ollama-chat:" + in.Model + ":" + strings.Join(parts, "|"), nil
}
