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
