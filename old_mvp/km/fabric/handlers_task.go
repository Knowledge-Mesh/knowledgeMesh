package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// TaskResult holds the result of an executeInference call, containing
// everything both Task and OpenAIChat handlers need to build their responses.
type TaskResult struct {
	TaskID         string
	WorkerResp     *WorkerResponse
	WinnerNode     *Node
	Charged        float64
	Refunded       float64
	ApiCost        float64
	SavingsPercent float64
	ModelVerified  string
	ActualTokens   int
}

// executeInference contains the shared logic for Task and OpenAIChat handlers:
// picking nodes, locking escrow, retry loop, forwarding, settling, computing savings.
func (h *Handlers) executeInference(taskID string, buyerName string, taskReq TaskRequest, logPrefix string) (*TaskResult, int, error) {
	// Get all eligible nodes sorted by price
	nodes := h.registry.OnlineNodes()
	eligible, escrowAmount, err := PickNodes(nodes, taskReq, h.capacity)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}

	log.Printf("[%s] %s: buyer=%s, %d eligible nodes, escrow=%.4f", logPrefix, taskID, buyerName, len(eligible), escrowAmount)

	// Lock escrow based on cheapest node
	if err := h.escrow.Lock(h.ledger, taskID, buyerName, escrowAmount); err != nil {
		return nil, http.StatusPaymentRequired, err
	}

	// Try nodes in order (cheapest first), retry on failure
	var workerResp *WorkerResponse
	var winnerNode *Node
	var lastErr error

	attempts := maxRetries
	if len(eligible) < attempts {
		attempts = len(eligible)
	}

	for i := 0; i < attempts; i++ {
		node := eligible[i]
		log.Printf("[%s] %s: attempt %d/%d → %s (%s)", logPrefix, taskID, i+1, attempts, node.Name, node.ID)

		h.capacity.AddInflight(node.ID)
		workerSecret, _ := h.state.GetNodeSecret(node.Name)
		resp, err := ForwardToWorker(node, taskReq, workerSecret)
		if err != nil {
			h.capacity.RemoveInflight(node.ID)
			lastErr = err
			log.Printf("[%s] %s: %s failed: %v — trying next node", logPrefix, taskID, node.Name, err)
			h.registry.MarkSuspect(node.ID)
			continue
		}

		workerResp = resp
		winnerNode = node
		h.capacity.RemoveInflight(node.ID)
		break
	}

	if workerResp == nil {
		log.Printf("[%s] %s: all %d attempts failed — releasing escrow", logPrefix, taskID, attempts)
		h.escrow.Release(h.ledger, taskID)
		return nil, http.StatusBadGateway, fmt.Errorf("all nodes failed after %d attempts: %v", attempts, lastErr)
	}

	// Model fingerprinting
	modelVerified := ""
	if workerResp.Model != "" && len(winnerNode.Models) > 0 {
		if _, ok := winnerNode.Models[workerResp.Model]; ok {
			modelVerified = "verified"
		} else {
			matched := false
			for advertised := range winnerNode.Models {
				if strings.HasPrefix(workerResp.Model, advertised) || strings.HasPrefix(advertised, workerResp.Model) {
					matched = true
					break
				}
			}
			if matched {
				modelVerified = "verified"
			} else {
				modelVerified = "mismatch"
				log.Printf("[%s] %s: MODEL MISMATCH — node %s advertises %v but returned model '%s'",
					logPrefix, taskID, winnerNode.Name, winnerNode.Models, workerResp.Model)
			}
		}
	} else if workerResp.Model == "" {
		modelVerified = "unverified"
	}

	// Server-side token sanity check
	actualTokens := workerResp.Usage.TotalTokens
	actualTokens = sanitizeTokenCount(actualTokens, taskReq.Messages, workerResp, taskID, logPrefix)

	// Settle with the node that succeeded — use per-model pricing if available
	effectivePrice := NodePrice(winnerNode, taskReq.Model)
	charged, refunded, err := h.escrow.Settle(h.ledger, taskID, winnerNode.Name, actualTokens, effectivePrice)
	if err != nil {
		log.Printf("[%s] %s: settlement error: %v", logPrefix, taskID, err)
		h.escrow.Release(h.ledger, taskID)
		return nil, http.StatusInternalServerError, fmt.Errorf("settlement error: %v", err)
	}

	// Record token usage for budget tracking
	h.capacity.RecordTokens(winnerNode.ID, int64(actualTokens))

	// Compute cost comparison
	apiRefPrice := apiReferencePrice(workerResp.Model)
	apiCost := float64(actualTokens) * apiRefPrice / 1_000_000
	savingsPercent := 0.0
	if apiCost > 0 {
		savingsPercent = (1 - charged/apiCost) * 100
	}

	log.Printf("[%s] %s: completed — model=%s (%s), %d tokens, charged=%.6f, saved=%.0f%% (via %s)",
		logPrefix, taskID, workerResp.Model, modelVerified, actualTokens, charged, savingsPercent, winnerNode.Name)
	h.syncToGitHub()

	return &TaskResult{
		TaskID:         taskID,
		WorkerResp:     workerResp,
		WinnerNode:     winnerNode,
		Charged:        charged,
		Refunded:       refunded,
		ApiCost:        apiCost,
		SavingsPercent: savingsPercent,
		ModelVerified:  modelVerified,
		ActualTokens:   actualTokens,
	}, http.StatusOK, nil
}

func (h *Handlers) Task(w http.ResponseWriter, r *http.Request) {
	if rateLimitByIP(h.limiters, r, 10, 20) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests. Slow down."})
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 100*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body (max 100KB)"})
		return
	}

	if req.Buyer == "" || len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "buyer and messages are required"})
		return
	}

	// Cap max_tokens to prevent abuse (buyer could set a huge value)
	if req.MaxTokens > maxMaxTokens {
		req.MaxTokens = maxMaxTokens
	}

	// Authenticate buyer with node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Buyer)
	if !hasSecret || !secretsEqual(req.BuyerSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid buyer_secret. Include your node secret to authenticate."})
		return
	}

	// Check buyer exists (must register via web dashboard first)
	if !h.ledger.UserExists(req.Buyer) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("user '%s' not registered. Register at %s first.", req.Buyer, r.Host),
		})
		return
	}

	taskID := fmt.Sprintf("task-%s", uuid.New().String()[:8])

	result, statusCode, err := h.executeInference(taskID, req.Buyer, req, "task")
	if err != nil {
		writeJSON(w, statusCode, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, TaskResponse{
		TaskID:         result.TaskID,
		AssignedTo:     result.WinnerNode.ID,
		WorkerName:     result.WinnerNode.Name,
		Result:         result.WorkerResp,
		CreditsCharged: result.Charged,
		EscrowRefunded: result.Refunded,
		ApiCost:        result.ApiCost,
		Savings:        result.SavingsPercent,
		ModelVerified:  result.ModelVerified,
	})
}

// OpenAI-compatible chat completions endpoint.
// Accepts standard OpenAI format so any tool (LangChain, LlamaIndex, Cursor, etc.)
// can use KnowledgeMesh as a drop-in backend.
func (h *Handlers) OpenAIChat(w http.ResponseWriter, r *http.Request) {
	// Extract Bearer token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid Authorization header. Use: Bearer <node_secret>"})
		return
	}
	bearerToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Look up buyer name from the secret (same logic as NodeConfig handler)
	buyerName := h.state.SecretForName(bearerToken)
	if buyerName == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bearer token"})
		return
	}

	// Parse the OpenAI-format request
	var openaiReq struct {
		Model            string           `json:"model"`
		Node             string           `json:"node,omitempty"`
		Messages         []Message        `json:"messages"`
		MaxTokens        int              `json:"max_tokens,omitempty"`
		Stream           bool             `json:"stream,omitempty"`
		WebSearchOptions *json.RawMessage `json:"web_search_options,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 100*1024)).Decode(&openaiReq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if len(openaiReq.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages are required"})
		return
	}

	// Cap max_tokens to prevent abuse
	if openaiReq.MaxTokens > maxMaxTokens {
		openaiReq.MaxTokens = maxMaxTokens
	}

	// Check buyer exists
	if !h.ledger.UserExists(buyerName) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("user '%s' not registered. Register at %s first.", buyerName, r.Host),
		})
		return
	}

	// Build a TaskRequest from the OpenAI request
	taskReq := TaskRequest{
		Buyer:            buyerName,
		BuyerSecret:      bearerToken,
		Messages:         openaiReq.Messages,
		Model:            openaiReq.Model,
		Node:             openaiReq.Node,
		MaxTokens:        openaiReq.MaxTokens,
		WebSearchOptions: openaiReq.WebSearchOptions,
	}

	taskID := fmt.Sprintf("km-%s", uuid.New().String()[:8])

	// ── Streaming path ──────────────────────────────────────────────
	if openaiReq.Stream {
		h.executeInferenceStreaming(w, taskID, buyerName, taskReq)
		return
	}

	// ── Non-streaming path (unchanged) ──────────────────────────────
	result, statusCode, err := h.executeInference(taskID, buyerName, taskReq, "openai")
	if err != nil {
		writeJSON(w, statusCode, map[string]string{"error": err.Error()})
		return
	}

	// Return response in OpenAI-compatible format
	writeJSON(w, http.StatusOK, WorkerResponse{
		ID:      result.TaskID,
		Object:  "chat.completion",
		Model:   result.WorkerResp.Model,
		Choices: result.WorkerResp.Choices,
		Usage:   result.WorkerResp.Usage,
	})
}

// executeInferenceStreaming handles stream:true requests. It picks a worker,
// locks escrow, streams SSE chunks from the worker back to the buyer, then
// settles escrow based on the final token count.
func (h *Handlers) executeInferenceStreaming(w http.ResponseWriter, taskID, buyerName string, taskReq TaskRequest) {
	logPrefix := "openai-stream"

	// Ensure the ResponseWriter supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Pick nodes + lock escrow (same as non-streaming)
	nodes := h.registry.OnlineNodes()
	eligible, escrowAmount, err := PickNodes(nodes, taskReq, h.capacity)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[%s] %s: buyer=%s, %d eligible nodes, escrow=%.4f", logPrefix, taskID, buyerName, len(eligible), escrowAmount)

	if err := h.escrow.Lock(h.ledger, taskID, buyerName, escrowAmount); err != nil {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": err.Error()})
		return
	}

	// Try nodes in order (cheapest first)
	var workerResp *http.Response
	var winnerNode *Node
	var lastErr error

	attempts := maxRetries
	if len(eligible) < attempts {
		attempts = len(eligible)
	}

	for i := 0; i < attempts; i++ {
		node := eligible[i]
		log.Printf("[%s] %s: attempt %d/%d → %s (%s)", logPrefix, taskID, i+1, attempts, node.Name, node.ID)

		h.capacity.AddInflight(node.ID)
		workerSecret, _ := h.state.GetNodeSecret(node.Name)
		resp, err := ForwardToWorkerStreaming(node, taskReq, workerSecret)
		if err != nil {
			h.capacity.RemoveInflight(node.ID)
			lastErr = err
			log.Printf("[%s] %s: %s failed: %v — trying next node", logPrefix, taskID, node.Name, err)
			h.registry.MarkSuspect(node.ID)
			continue
		}

		workerResp = resp
		winnerNode = node
		break
	}

	if workerResp == nil {
		log.Printf("[%s] %s: all %d attempts failed — releasing escrow", logPrefix, taskID, attempts)
		h.escrow.Release(h.ledger, taskID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("all nodes failed after %d attempts: %v", attempts, lastErr)})
		return
	}

	defer func() {
		workerResp.Body.Close()
		h.capacity.RemoveInflight(winnerNode.ID)
	}()

	// Set SSE headers before writing the first byte
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Pipe SSE events from worker to client, collecting usage from the last data chunk.
	var finalUsage Usage
	var modelFromStream string

	scanner := bufio.NewScanner(workerResp.Body)
	// Increase buffer for potentially large SSE lines
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Forward every line (including blank lines that delimit SSE events)
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()

		// Parse "data: ..." lines to extract usage from the final chunk
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}

			// Try to parse the chunk to extract model and usage info
			var chunk struct {
				Model string `json:"model,omitempty"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage,omitempty"`
			}
			if json.Unmarshal([]byte(data), &chunk) == nil {
				if chunk.Model != "" {
					modelFromStream = chunk.Model
				}
				if chunk.Usage != nil {
					finalUsage = Usage{
						PromptTokens:     chunk.Usage.PromptTokens,
						CompletionTokens: chunk.Usage.CompletionTokens,
						TotalTokens:      chunk.Usage.TotalTokens,
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] %s: stream read error: %v", logPrefix, taskID, err)
		// Stream may be partially sent; write an error event
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errJSON)
		flusher.Flush()
	}

	// ── Settlement ──────────────────────────────────────────────────
	// If the worker provided usage in the stream, use it; otherwise estimate.
	actualTokens := finalUsage.TotalTokens
	if actualTokens <= 0 {
		// Estimate from request messages (output unknown — use conservative 500)
		inputChars := 0
		for _, m := range taskReq.Messages {
			inputChars += len(m.Content)
		}
		actualTokens = EstimateTokens(string(make([]byte, inputChars))) + 500
		log.Printf("[%s] %s: no usage in stream, estimating %d tokens", logPrefix, taskID, actualTokens)
	}

	// Build a minimal WorkerResponse for sanitizeTokenCount
	dummyResp := &WorkerResponse{
		Model: modelFromStream,
		Usage: finalUsage,
	}
	actualTokens = sanitizeTokenCount(actualTokens, taskReq.Messages, dummyResp, taskID, logPrefix)

	effectivePrice := NodePrice(winnerNode, taskReq.Model)
	charged, _, err := h.escrow.Settle(h.ledger, taskID, winnerNode.Name, actualTokens, effectivePrice)
	if err != nil {
		log.Printf("[%s] %s: settlement error: %v", logPrefix, taskID, err)
		h.escrow.Release(h.ledger, taskID)
		return
	}

	h.capacity.RecordTokens(winnerNode.ID, int64(actualTokens))

	apiRefPrice := apiReferencePrice(modelFromStream)
	apiCost := float64(actualTokens) * apiRefPrice / 1_000_000
	savingsPercent := 0.0
	if apiCost > 0 {
		savingsPercent = (1 - charged/apiCost) * 100
	}

	log.Printf("[%s] %s: completed — model=%s, %d tokens, charged=%.6f, saved=%.0f%% (via %s)",
		logPrefix, taskID, modelFromStream, actualTokens, charged, savingsPercent, winnerNode.Name)
	h.syncToGitHub()
}
