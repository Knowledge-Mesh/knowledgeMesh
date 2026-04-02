package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultOllamaURL     = "http://127.0.0.1:11434"
	defaultOllamaTimeout = 120 * time.Second
)

// HTTPBackend is a real Ollama HTTP client that talks to a running Ollama server.
type HTTPBackend struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPBackend creates a backend pointed at the given base URL.
// If baseURL is empty, defaults to http://127.0.0.1:11434.
func NewHTTPBackend(baseURL string) *HTTPBackend {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOllamaURL
	}
	return &HTTPBackend{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: defaultOllamaTimeout},
	}
}

func (h *HTTPBackend) http() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}
	return &http.Client{Timeout: defaultOllamaTimeout}
}

// ── /api/tags ───────────────────────────────────────────────────────

// ListModels calls GET /api/tags and returns installed models.
func (h *HTTPBackend) ListModels(ctx context.Context) (ListModelsResponse, error) {
	var zero ListModelsResponse

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+"/api/tags", nil)
	if err != nil {
		return zero, err
	}

	resp, err := h.http().Do(req)
	if err != nil {
		return zero, fmt.Errorf("cannot reach ollama at %s: %w", h.BaseURL, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("ollama /api/tags %s: %s", resp.Status, truncateForErr(b))
	}

	var out ListModelsResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// ── /api/generate ───────────────────────────────────────────────────

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
}

// Generate calls POST /api/generate with stream: false.
func (h *HTTPBackend) Generate(ctx context.Context, in GenerateInput) (string, error) {
	body := generateRequest{
		Model:  in.Model,
		Prompt: in.Prompt,
		Stream: false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/api/generate", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama /api/generate: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama /api/generate %s: %s", resp.Status, truncateForErr(b))
	}

	var out generateResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return out.Response, nil
}

// ── /api/chat ───────────────────────────────────────────────────────

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Model   string      `json:"model"`
	Message ChatMessage `json:"message"`
}

// Chat calls POST /api/chat with stream: false.
func (h *HTTPBackend) Chat(ctx context.Context, in ChatInput) (string, error) {
	body := chatRequest{
		Model:    in.Model,
		Messages: in.Messages,
		Stream:   false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama /api/chat: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama /api/chat %s: %s", resp.Status, truncateForErr(b))
	}

	var out chatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}

// ── helpers ─────────────────────────────────────────────────────────

func truncateForErr(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
