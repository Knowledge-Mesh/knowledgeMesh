package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

const (
	defaultBaseURL     = "https://api.openai.com/v1"
	defaultHTTPTimeout = 120 * time.Second
)

var ErrMissingAPIKey = errors.New("openai API key not found in configured env")

// Client performs stateless OpenAI Chat Completions calls (no conversation history beyond this request).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKeyEnv  string
}

func NewClientFromConfig(cfg *types.OpenAISellerConfig) *Client {
	if cfg == nil {
		return &Client{BaseURL: defaultBaseURL, HTTPClient: &http.Client{Timeout: defaultHTTPTimeout}}
	}
	env := strings.TrimSpace(cfg.APIKeyEnv)
	if env == "" {
		env = "OPENAI_API_KEY"
	}
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = defaultBaseURL
	}
	return &Client{
		BaseURL:    strings.TrimRight(base, "/"),
		HTTPClient: &http.Client{Timeout: defaultHTTPTimeout},
		APIKeyEnv:  env,
	}
}

func (c *Client) apiKey() (string, error) {
	if c.APIKeyEnv == "" {
		c.APIKeyEnv = "OPENAI_API_KEY"
	}
	v := strings.TrimSpace(os.Getenv(c.APIKeyEnv))
	if v == "" {
		return "", ErrMissingAPIKey
	}
	return v, nil
}

type chatCompletionRequest struct {
	Model    string            `json:"model"`
	Messages []chatMessagePart `json:"messages"`
}

type chatMessagePart struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ChatCompletion sends one request with the given messages only (no stored history).
func (c *Client) ChatCompletion(ctx context.Context, model string, messages []chatMessagePart) (chatCompletionResponse, error) {
	var zero chatCompletionResponse
	key, err := c.apiKey()
	if err != nil {
		return zero, err
	}
	body := chatCompletionRequest{Model: model, Messages: messages}
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.http().Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("openai api %s: %s", resp.Status, truncateForErr(b))
	}
	var out chatCompletionResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func truncateForErr(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
