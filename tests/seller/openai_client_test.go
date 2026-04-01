package seller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/openai"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestOpenAISellerFacadeGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("expected single message, got %v", msgs)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-1",
			"model": "gpt-4o-mini",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "done"}},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg := &types.OpenAISellerConfig{APIKeyEnv: "OPENAI_API_KEY", BaseURL: srv.URL + "/v1"}
	f := openai.NewSellerFacade(cfg)

	text, _, err := f.Generate(context.Background(), openai.GenerateInput{
		Model:  "gpt-4o-mini",
		Prompt: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "done" {
		t.Fatalf("got %q", text)
	}
}

func TestOpenAIEngineResolveModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "x"}}},
		})
	}))
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", "k")

	cfg := &types.OpenAISellerConfig{
		APIKeyEnv: "OPENAI_API_KEY",
		BaseURL:   srv.URL + "/v1",
		Models: []types.OpenAIModelDecl{
			{ID: "my-gpt", Name: "gpt-4o-mini"},
		},
	}
	eng := openai.NewEngine(cfg)
	out, err := eng.Generate(context.Background(), "prompt", types.InferenceRequest{ModelName: "my-gpt"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "x" {
		t.Fatalf("got %q", out)
	}
}
