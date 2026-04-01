package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/api"
)

func TestAnthropicMessagesParsesAndFormatsResponse(t *testing.T) {
	t.Parallel()

	srv := api.NewServer(":0", nil)
	payload := map[string]any{
		"model": "claude-mock",
		"messages": []map[string]any{
			{"role": "user", "content": "hello from anthropic"},
		},
		"max_tokens": 128,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: got %d body=%s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["id"]; !ok {
		t.Fatalf("missing id")
	}
	if body["type"] != "message" {
		t.Fatalf("expected type=message, got %v", body["type"])
	}
	if body["role"] != "assistant" {
		t.Fatalf("expected role=assistant, got %v", body["role"])
	}
	if body["model"] != "claude-mock" {
		t.Fatalf("model mismatch: got %v", body["model"])
	}
	if _, ok := body["content"]; !ok {
		t.Fatalf("missing content")
	}
	if _, ok := body["usage"]; !ok {
		t.Fatalf("missing usage")
	}
}

func TestAnthropicMessagesParsesContentBlockArray(t *testing.T) {
	t.Parallel()

	srv := api.NewServer(":0", nil)
	raw := `{
	  "model":"claude-mock",
	  "messages":[
	    {"role":"user","content":[{"type":"text","text":"block text input"}]}
	  ],
	  "max_tokens":64
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: got %d body=%s", rr.Code, rr.Body.String())
	}
}
