package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// MeshRuntime is implemented by mesh.Runtime: buyer auth + remote inference.
type MeshRuntime interface {
	RunInference(ctx context.Context, sessionID string, req types.InferenceRequest) (types.InferenceResponse, error)
	Register(name, email, password string) (buyer.State, error)
	Login(userOrEmail, password string) (buyer.State, error)
}

type Server struct {
	Addr string
	Mesh MeshRuntime
}

func NewServer(addr string, mesh MeshRuntime) *Server {
	return &Server{Addr: addr, Mesh: mesh}
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
	mux.HandleFunc("/api/v1/buyer/register", s.handleBuyerRegister)
	mux.HandleFunc("/api/v1/buyer/login", s.handleBuyerLogin)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/messages", s.handleAnthropicMessages)
	return mux
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, s.Handler())
}

var errNoSession = errors.New("missing session")

func (s *Server) parseSession(r *http.Request) (string, error) {
	if h := strings.TrimSpace(r.Header.Get("X-Session-ID")); h != "" {
		return h, nil
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:]), nil
	}
	return "", errNoSession
}

type buyerRegisterBody struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type buyerLoginBody struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

func (s *Server) handleBuyerRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Mesh == nil {
		http.Error(w, "buyer API not enabled", http.StatusNotImplemented)
		return
	}
	var body buyerRegisterBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.TrimSpace(body.Username)
	}
	if name == "" {
		writeOpenAIError(w, http.StatusBadRequest, "name or username is required")
		return
	}
	st, err := s.Mesh.Register(name, body.Email, body.Password)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"buyerId": st.BuyerID})
}

func (s *Server) handleBuyerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Mesh == nil {
		http.Error(w, "buyer API not enabled", http.StatusNotImplemented)
		return
	}
	var body buyerLoginBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	st, err := s.Mesh.Login(body.User, body.Password)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"sessionId": st.SessionID,
		"buyerId":   st.BuyerID,
	})
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
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body")
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
		Skill: types.Skill{
			Name:       "chat",
			ModelName:  req.Model,
			ModelType:  "llm",
			TuningTier: "base",
		},
	}

	var internalResp types.InferenceResponse
	var err error
	if s.Mesh != nil {
		sess, errSess := s.parseSession(r)
		if errSess != nil {
			writeOpenAIError(w, http.StatusUnauthorized, "missing X-Session-ID or Bearer token")
			return
		}
		internalResp, err = s.Mesh.RunInference(r.Context(), sess, internalReq)
	} else {
		internalResp = runMockInference(internalReq)
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, buyer.ErrInvalidSession) {
			status = http.StatusUnauthorized
		}
		writeOpenAIError(w, status, err.Error())
		return
	}
	if !internalResp.Success {
		msg := internalResp.Error
		if msg == "" {
			msg = "inference failed"
		}
		writeOpenAIError(w, http.StatusBadGateway, msg)
		return
	}

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
		writeAnthropicError(w, http.StatusBadRequest, "invalid JSON body")
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
		Skill: types.Skill{
			Name:       "chat",
			ModelName:  req.Model,
			ModelType:  "llm",
			TuningTier: "base",
		},
	}

	var internalResp types.InferenceResponse
	var err error
	if s.Mesh != nil {
		sess, errSess := s.parseSession(r)
		if errSess != nil {
			writeAnthropicError(w, http.StatusUnauthorized, "missing X-Session-ID or Bearer token")
			return
		}
		internalResp, err = s.Mesh.RunInference(r.Context(), sess, internalReq)
	} else {
		internalResp = runMockInference(internalReq)
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, buyer.ErrInvalidSession) {
			status = http.StatusUnauthorized
		}
		writeAnthropicError(w, status, err.Error())
		return
	}
	if !internalResp.Success {
		msg := internalResp.Error
		if msg == "" {
			msg = "inference failed"
		}
		writeAnthropicError(w, http.StatusBadGateway, msg)
		return
	}

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

func writeOpenAIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": msg,
			"type":    "invalid_request_error",
		},
	})
}

func writeAnthropicError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"type":    "api_error",
			"message": msg,
		},
	})
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
