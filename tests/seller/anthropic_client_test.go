package seller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/anthropic"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestClientCreateMessageNoHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
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
			"id":   "msg_1",
			"type": "message",
			"role": "assistant",
			"content": []map[string]string{
				{"type": "text", "text": "hi"},
			},
			"model": "claude-test",
			"usage": map[string]int{"input_tokens": 3, "output_tokens": 2},
		})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := &types.AnthropicSellerConfig{APIKeyEnv: "ANTHROPIC_API_KEY"}
	c := anthropic.NewClientFromConfig(cfg)
	c.BaseURL = srv.URL

	out, err := c.CreateMessage(context.Background(), "claude-test", "hello", 64)
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if out.Content[0].Text != "hi" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSellerFacadeGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "done"}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "k")

	cfg := &types.AnthropicSellerConfig{APIKeyEnv: "ANTHROPIC_API_KEY"}
	f := anthropic.NewSellerFacade(cfg)
	f.BaseURL = srv.URL

	text, _, err := f.Generate(context.Background(), anthropic.GenerateInput{
		Model:  "m",
		Prompt: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "done" {
		t.Fatalf("got %q", text)
	}
}
