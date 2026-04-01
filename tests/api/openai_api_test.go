package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/api"
)

func TestGetModelsOpenAIShape(t *testing.T) {
	t.Parallel()

	srv := api.NewServer(":0", nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["object"] != "list" {
		t.Fatalf("expected object=list, got %v", body["object"])
	}
	if _, ok := body["data"]; !ok {
		t.Fatalf("missing data field")
	}
}

func TestChatCompletionsOpenAIShape(t *testing.T) {
	t.Parallel()

	srv := api.NewServer(":0", nil)
	payload := map[string]any{
		"model": "kmg-mock-1",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
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
	if body["object"] != "chat.completion" {
		t.Fatalf("object mismatch: got %v", body["object"])
	}
	if body["model"] != "kmg-mock-1" {
		t.Fatalf("model mismatch: got %v", body["model"])
	}
	if _, ok := body["choices"]; !ok {
		t.Fatalf("missing choices")
	}
	if _, ok := body["usage"]; !ok {
		t.Fatalf("missing usage")
	}
}
