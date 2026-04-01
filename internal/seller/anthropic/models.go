package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListModelsResponse is a minimal Anthropic-compatible list models shape.
type ListModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Display string `json:"display_name,omitempty"`
	} `json:"data"`
	FirstID string `json:"first_id,omitempty"`
	HasMore bool   `json:"has_more,omitempty"`
}

// ListModels calls GET /v1/models (requires API key).
func (c *Client) ListModels(ctx context.Context) (ListModelsResponse, error) {
	var zero ListModelsResponse
	key, err := c.apiKey()
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/v1/models", nil)
	if err != nil {
		return zero, err
	}
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
		return zero, fmt.Errorf("anthropic list models %s: %s", resp.Status, truncateForErr(b))
	}
	var out ListModelsResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, err
	}
	return out, nil
}
