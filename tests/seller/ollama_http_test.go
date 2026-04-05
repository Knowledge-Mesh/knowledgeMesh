package seller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller/ollama"
)

func TestHTTPBackendListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(ollama.ListModelsResponse{
			Models: []ollama.ModelInfo{
				{Name: "llama3.2:latest"},
				{Name: "phi3:latest"},
			},
		})
	}))
	defer srv.Close()

	b := ollama.NewHTTPBackend(srv.URL)
	resp, err := b.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}
	if resp.Models[0].Name != "llama3.2:latest" {
		t.Fatalf("expected llama3.2:latest, got %s", resp.Models[0].Name)
	}
}

func TestHTTPBackendGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Stream {
			t.Fatal("expected stream: false")
		}
		if req.Model != "llama3.2:latest" {
			t.Fatalf("expected model llama3.2:latest, got %s", req.Model)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"model":    req.Model,
			"response": "The capital of Japan is Tokyo.",
		})
	}))
	defer srv.Close()

	b := ollama.NewHTTPBackend(srv.URL)
	out, err := b.Generate(context.Background(), ollama.GenerateInput{
		Model:  "llama3.2:latest",
		Prompt: "What is the capital of Japan?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "The capital of Japan is Tokyo." {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestHTTPBackendChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model    string               `json:"model"`
			Messages []ollama.ChatMessage `json:"messages"`
			Stream   bool                 `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Stream {
			t.Fatal("expected stream: false")
		}
		if len(req.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "user" {
			t.Fatalf("expected role user, got %s", req.Messages[0].Role)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"model": req.Model,
			"message": ollama.ChatMessage{
				Role:    "assistant",
				Content: "Hello! How can I help you?",
			},
		})
	}))
	defer srv.Close()

	b := ollama.NewHTTPBackend(srv.URL)
	out, err := b.Chat(context.Background(), ollama.ChatInput{
		Model: "llama3.2:latest",
		Messages: []ollama.ChatMessage{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hello! How can I help you?" {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestHTTPBackendServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model 'nonexistent' not found"}`))
	}))
	defer srv.Close()

	b := ollama.NewHTTPBackend(srv.URL)
	_, err := b.Generate(context.Background(), ollama.GenerateInput{
		Model:  "nonexistent",
		Prompt: "hi",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestHTTPBackendUnreachable(t *testing.T) {
	b := ollama.NewHTTPBackend("http://127.0.0.1:1") // nothing listening
	_, err := b.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
