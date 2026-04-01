package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListModelsResponse is a minimal OpenAI-compatible list models shape.
type ListModelsResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// ListModels calls GET /v1/models (requires API key).
func (c *Client) ListModels(ctx context.Context) (ListModelsResponse, error) {
	var zero ListModelsResponse
	key, err := c.apiKey()
	if err != nil {
		return zero, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
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
		return zero, fmt.Errorf("openai list models %s: %s", resp.Status, truncateForErr(b))
	}
	var out ListModelsResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, err
	}
	return out, nil
}
