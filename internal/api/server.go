package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

type Server struct {
	Addr string
}

func NewServer(addr string) *Server {
	return &Server{Addr: addr}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"module": "knowledgeMesh",
		})
	})
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/messages", s.handleAnthropicMessages)
	return mux
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, s.Handler())
}

type openAIModelsResponse struct {
	Object string            `json:"object"`
	Data   []openAIModelItem `json:"data"`
}

type openAIModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type openAIChatCompletionsRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionsResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []openAIChoice         `json:"choices"`
	Usage   openAICompletionUsages `json:"usage"`
}

type openAIChoice struct {
	Index        int               `json:"index"`
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type openAICompletionUsages struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicMessagesResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicMessagesUsage  `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicMessagesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openAIModelsResponse{
		Object: "list",
		Data: []openAIModelItem{
			{
				ID:      "kmg-mock-1",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "knowledgeMesh",
			},
		},
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req openAIChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "kmg-mock-1"
	}
	prompt := flattenMessages(req.Messages)
	internalReq := types.InferenceRequest{
		RequestID:  fmt.Sprintf("req-%d", time.Now().UnixNano()),
		ModelName:  req.Model,
		ModelType:  "llm",
		TuningTier: "base",
		Input:      prompt,
	}

	internalResp := runMockInference(internalReq)

	resp := openAIChatCompletionsResponse{
		ID:      "chatcmpl-" + internalResp.RequestID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   internalResp.ModelName,
		Choices: []openAIChoice{
			{
				Index: 0,
				Message: openAIChatMessage{
					Role:    "assistant",
					Content: internalResp.Output,
				},
				FinishReason: "stop",
			},
		},
		Usage: openAICompletionUsages{
			PromptTokens:     internalResp.TokenUsage.InputTokens,
			CompletionTokens: internalResp.TokenUsage.OutputTokens,
			TotalTokens:      internalResp.TokenUsage.TotalTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req anthropicMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "kmg-mock-1"
	}
	prompt := flattenAnthropicMessages(req.Messages)
	internalReq := types.InferenceRequest{
		RequestID:  fmt.Sprintf("req-%d", time.Now().UnixNano()),
		ModelName:  req.Model,
		ModelType:  "llm",
		TuningTier: "base",
		Input:      prompt,
	}
	internalResp := runMockInference(internalReq)

	resp := anthropicMessagesResponse{
		ID:   "msg_" + internalResp.RequestID,
		Type: "message",
		Role: "assistant",
		Content: []anthropicContentBlock{
			{Type: "text", Text: internalResp.Output},
		},
		Model:      internalResp.ModelName,
		StopReason: "end_turn",
		Usage: anthropicMessagesUsage{
			InputTokens:  internalResp.TokenUsage.InputTokens,
			OutputTokens: internalResp.TokenUsage.OutputTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func flattenMessages(messages []openAIChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	lines := make([]string, 0, len(messages))
	for _, m := range messages {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		lines = append(lines, m.Content)
	}
	return strings.Join(lines, "\n")
}

func flattenAnthropicMessages(messages []anthropicMessage) string {
	lines := make([]string, 0, len(messages))
	for _, m := range messages {
		text := extractAnthropicText(m.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func extractAnthropicText(raw json.RawMessage) string {
	// Basic compatibility: support either string content or first text block.
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if t, ok := b["type"].(string); ok && t == "text" {
				if txt, ok := b["text"].(string); ok {
					return txt
				}
			}
		}
	}
	return ""
}

func runMockInference(req types.InferenceRequest) types.InferenceResponse {
	output := "mock-result:" + req.Input
	inputTokens := estimateTokens(req.Input)
	outputTokens := estimateTokens(output)
	return types.InferenceResponse{
		RequestID:    req.RequestID,
		SellerPeerID: "local-seller",
		Status:       "success",
		ModelName:    req.ModelName,
		ModelType:    req.ModelType,
		TuningTier:   req.TuningTier,
		Output:       output,
		Success:      true,
		TokenUsage: types.UsageCounters{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
	}
}

func estimateTokens(v string) int {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	return len(strings.Fields(v))
}
