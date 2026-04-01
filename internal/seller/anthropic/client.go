package anthropic

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
	defaultBaseURL      = "https://api.anthropic.com"
	defaultAnthropicVer = "2023-06-01"
	defaultHTTPTimeout  = 120 * time.Second
)

var ErrMissingAPIKey = errors.New("anthropic API key not found in configured env")

// Client performs stateless Anthropic Messages API calls (no conversation history).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKeyEnv  string
}

func NewClientFromConfig(cfg *types.AnthropicSellerConfig) *Client {
	if cfg == nil {
		return &Client{BaseURL: defaultBaseURL, HTTPClient: &http.Client{Timeout: defaultHTTPTimeout}}
	}
	env := strings.TrimSpace(cfg.APIKeyEnv)
	if env == "" {
		env = "ANTHROPIC_API_KEY"
	}
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: defaultHTTPTimeout},
		APIKeyEnv:  env,
	}
}

func (c *Client) apiKey() (string, error) {
	if c.APIKeyEnv == "" {
		c.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	v := strings.TrimSpace(os.Getenv(c.APIKeyEnv))
	if v == "" {
		return "", ErrMissingAPIKey
	}
	return v, nil
}

// messagesRequest is a minimal Anthropic Messages API body (single user turn, no history).
type messagesRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Messages  []messageContent `json:"messages"`
}

type messageContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   messagesUsage  `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type messagesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CreateMessage sends one user message; no prior context is included.
func (c *Client) CreateMessage(ctx context.Context, model string, userText string, maxTokens int) (messagesResponse, error) {
	var zero messagesResponse
	key, err := c.apiKey()
	if err != nil {
		return zero, err
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	body := messagesRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages: []messageContent{
			{Role: "user", Content: userText},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", defaultAnthropicVer)

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
		return zero, fmt.Errorf("anthropic api %s: %s", resp.Status, truncateForErr(b))
	}
	var out messagesResponse
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
