package seller_test

import (
	"context"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestOllamaMockListGenerateChat(t *testing.T) {
	f := ollama.NewSellerFacade(nil, nil)
	ctx := context.Background()

	list, err := f.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Models) < 1 {
		t.Fatal("expected models")
	}

	out, err := f.Generate(ctx, ollama.GenerateInput{Model: "llama3:mock", Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty generate")
	}

	chatOut, err := f.Chat(ctx, ollama.ChatInput{
		Model: "m",
		Messages: []ollama.ChatMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chatOut == "" {
		t.Fatal("empty chat")
	}
}

func TestOllamaEngineResolveModel(t *testing.T) {
	cfg := &types.OllamaSellerConfig{
		Models: []types.OllamaModelDecl{
			{ID: "local", Name: "llama3:latest"},
		},
	}
	eng := ollama.NewEngine(cfg, nil)
	out, err := eng.Generate(context.Background(), "p", types.InferenceRequest{ModelName: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty output")
	}
}
