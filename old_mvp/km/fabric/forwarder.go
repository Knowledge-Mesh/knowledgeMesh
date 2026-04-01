package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// forwardClient uses a custom transport with short DNS TTL and no idle
// connection caching, so that tunnel URL changes (new cloudflared process
// → new DNS entry) are picked up immediately.
var forwardClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 0, // disable keep-alive to force fresh DNS
		}).DialContext,
		DisableKeepAlives:   true,  // no connection reuse — fresh DNS every request
		MaxIdleConns:        0,
		IdleConnTimeout:     0,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// streamingForwardClient is like forwardClient but without a global timeout
// so that long-running SSE streams are not prematurely closed.
var streamingForwardClient = &http.Client{
	Timeout: 0, // no timeout — the caller controls the lifetime
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		DisableKeepAlives:   false,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// ForwardRequest is the OpenAI-compatible payload sent to the worker.
type ForwardRequest struct {
	Model            string           `json:"model,omitempty"`
	Messages         []Message        `json:"messages"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	Temperature      *float32         `json:"temperature,omitempty"`
	Stream           bool             `json:"stream,omitempty"`
	WebSearchOptions *json.RawMessage `json:"web_search_options,omitempty"`
}

// signBody computes HMAC-SHA256 of body using the given secret and returns the hex-encoded signature.
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ForwardToWorker sends the inference request to a worker's tunnel URL
// and returns the parsed response. If nodeSecret is non-empty, the request
// body is signed with HMAC-SHA256 and sent in the X-KM-Signature header.
func ForwardToWorker(node *Node, req TaskRequest, nodeSecret string) (*WorkerResponse, error) {
	payload := ForwardRequest{
		Model:            req.Model,
		Messages:         req.Messages,
		WebSearchOptions: req.WebSearchOptions,
	}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		payload.MaxTokens = &mt
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal forward request: %w", err)
	}

	url := node.TunnelURL + "/v1/chat/completions"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if nodeSecret != "" {
		httpReq.Header.Set("X-KM-Signature", signBody(nodeSecret, body))
	}

	resp, err := forwardClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("forward to worker %s: %w", node.Name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read worker response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		rawErr := string(respBody)
		if isWorkerAuthError(resp.StatusCode, rawErr) {
			return nil, fmt.Errorf("worker '%s' is temporarily unavailable. Try a different node or model.", node.Name)
		}
		return nil, fmt.Errorf("worker %s returned %d: %s", node.Name, resp.StatusCode, rawErr)
	}

	// Sanitize control characters that may appear unescaped in JSON string
	// values from worker responses (e.g. search result snippets).
	respBody = sanitizeJSONBytes(respBody)

	var workerResp WorkerResponse
	if err := json.Unmarshal(respBody, &workerResp); err != nil {
		return nil, fmt.Errorf("parse worker response: %w", err)
	}

	return &workerResp, nil
}

// sanitizeJSONBytes replaces control characters (0x00-0x1F) that are not
// valid unescaped inside JSON strings with spaces. It only operates inside
// JSON string values (between unescaped double quotes) to avoid corrupting
// the JSON structure.
func sanitizeJSONBytes(data []byte) []byte {
	result := make([]byte, len(data))
	copy(result, data)

	inString := false
	for i := 0; i < len(result); i++ {
		b := result[i]
		if inString {
			if b == '\\' {
				i++ // skip escaped character
				continue
			}
			if b == '"' {
				inString = false
				continue
			}
			// Replace control chars inside strings (except \t \n \r which
			// json.Unmarshal can handle when escaped, but are invalid raw)
			if b < 0x20 {
				result[i] = ' '
			}
		} else {
			if b == '"' {
				inString = true
			}
		}
	}
	return result
}

// ForwardToWorkerStreaming sends the inference request with stream:true and
// returns the raw HTTP response so the caller can pipe SSE events back to the
// client. The caller is responsible for closing resp.Body.
func ForwardToWorkerStreaming(node *Node, req TaskRequest, nodeSecret string) (*http.Response, error) {
	payload := ForwardRequest{
		Model:            req.Model,
		Messages:         req.Messages,
		Stream:           true,
		WebSearchOptions: req.WebSearchOptions,
	}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		payload.MaxTokens = &mt
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal forward request: %w", err)
	}

	url := node.TunnelURL + "/v1/chat/completions"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if nodeSecret != "" {
		httpReq.Header.Set("X-KM-Signature", signBody(nodeSecret, body))
	}

	resp, err := streamingForwardClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("forward streaming to worker %s: %w", node.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		rawErr := string(respBody)
		if isWorkerAuthError(resp.StatusCode, rawErr) {
			return nil, fmt.Errorf("worker '%s' is temporarily unavailable. Try a different node or model.", node.Name)
		}
		return nil, fmt.Errorf("worker %s returned %d: %s", node.Name, resp.StatusCode, rawErr)
	}

	return resp, nil
}

// isWorkerAuthError detects if a worker error is caused by an expired session
// or auth issue. We check for 500 status with auth-related keywords so the
// broker can return a sanitized message to the buyer instead of leaking
// internal worker details.
func isWorkerAuthError(statusCode int, body string) bool {
	if statusCode == http.StatusInternalServerError || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		lower := strings.ToLower(body)
		return strings.Contains(lower, "session expired") ||
			strings.Contains(lower, "session key") ||
			strings.Contains(lower, "auth_error") ||
			strings.Contains(lower, "cookie expired") ||
			strings.Contains(lower, "unauthorized") ||
			strings.Contains(lower, "authentication")
	}
	return false
}
