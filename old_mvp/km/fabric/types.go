package main

import (
	"encoding/json"
	"time"
)

// ── Node ─────────────────────────────────────────────────────────────

type Node struct {
	ID               string             `json:"node_id"`
	Name             string             `json:"name"`
	Tier             string             `json:"tier"` // "api", "subscription", "local"
	PricePerM        float64            `json:"price_per_million_tokens"`     // legacy flat price
	Models           map[string]float64 `json:"models,omitempty"`             // model → price/M tokens
	TunnelURL        string             `json:"tunnel_url"`
	LastHeartbeat    time.Time          `json:"last_heartbeat"`
	Status           string             `json:"status"` // "online", "offline"
	TokenBudget      int64              `json:"token_budget,omitempty"`       // max tokens in rolling window (0 = unlimited)
	BudgetWindowHours int              `json:"budget_window_hours,omitempty"` // rolling window in hours (0 = unlimited)
	MaxConcurrent    int                `json:"max_concurrent,omitempty"`     // max inflight requests (0 = tier default)
	Activated        bool               `json:"activated"`                    // true after first worker connection
}

// ── Registration ─────────────────────────────────────────────────────

type RegisterRequest struct {
	Name              string             `json:"name"`
	Tier              string             `json:"tier"`
	PricePerM         float64            `json:"price_per_million_tokens"`
	Models            map[string]float64 `json:"models,omitempty"` // model → price/M tokens
	TunnelURL         string             `json:"tunnel_url"`
	NodeSecret        string             `json:"node_secret,omitempty"`
	TokenBudget       int64              `json:"token_budget,omitempty"`
	BudgetWindowHours int                `json:"budget_window_hours,omitempty"`
	MaxConcurrent     int                `json:"max_concurrent,omitempty"`
}

type RegisterResponse struct {
	NodeID  string `json:"node_id"`
	Message string `json:"message"`
}

// ── Heartbeat ────────────────────────────────────────────────────────

type HeartbeatRequest struct {
	NodeID     string `json:"node_id"`
	NodeSecret string `json:"node_secret,omitempty"`
}

// ── Task ─────────────────────────────────────────────────────────────

type TaskRequest struct {
	Buyer          string    `json:"buyer"`
	BuyerSecret    string    `json:"buyer_secret"`
	Messages       []Message `json:"messages"`
	Model          string    `json:"model,omitempty"`
	Node           string    `json:"node,omitempty"`           // pin to a specific worker by name
	TierPreference string    `json:"tier_preference,omitempty"`
	MaxBudget      float64   `json:"max_budget"`
	MaxTokens        int              `json:"max_tokens,omitempty"`     // buyer-specified max tokens for accurate escrow
	WebSearchOptions *json.RawMessage `json:"web_search_options,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TaskResponse struct {
	TaskID         string      `json:"task_id"`
	AssignedTo     string      `json:"assigned_to"`
	WorkerName     string      `json:"worker_name"`
	Result         interface{} `json:"result"`
	CreditsCharged float64     `json:"credits_charged"`
	EscrowRefunded float64     `json:"escrow_refunded"`
	ApiCost        float64     `json:"api_cost"`                   // what this would cost on direct API
	Savings        float64     `json:"savings_percent"`             // percentage saved vs API
	ModelVerified  string      `json:"model_verified,omitempty"`   // "verified", "unverified", "mismatch"
}

// Reference API pricing (blended input+output average per model)
const ApiReferencePerM = 15.0 // default fallback: $15/M tokens

// apiReferencePrice returns the retail API price for a given model.
// Used to calculate savings vs direct API.
func apiReferencePrice(model string) float64 {
	// Known model prices (blended input+output per million tokens)
	prices := map[string]float64{
		// Anthropic
		"claude-sonnet-4-20250514": 12.0,    // $3 in + $15 out, blended
		"claude-haiku-4-20250514":  2.5,     // $0.80 in + $4 out, blended
		"claude-opus-4-20250514":   45.0,    // $15 in + $75 out, blended
		// OpenAI
		"gpt-4o":                   7.5,     // $2.50 in + $10 out, blended
		"gpt-4o-2024-08-06":        7.5,
		"gpt-4o-mini":              0.375,   // $0.15 in + $0.60 out, blended
		"gpt-4o-mini-2024-07-18":   0.375,
		"gpt-4.1":                  10.0,    // $2 in + $8 out, blended (estimated)
		"gpt-4.1-mini":             1.6,
		"gpt-4.1-nano":             0.4,
		// Local models (no API cost — use a small reference for savings calc)
		"llama3.2":                 0.0,
		"llama3":                   0.0,
		"mistral":                  0.0,
	}

	// Exact match
	if p, ok := prices[model]; ok {
		if p == 0 {
			return 0.10 // local models: use $0.10/M as reference (cloud hosting cost)
		}
		return p
	}

	// Prefix match
	for prefix, p := range prices {
		if len(model) > 0 && len(prefix) > 0 {
			if (len(model) >= len(prefix) && model[:len(prefix)] == prefix) ||
				(len(prefix) >= len(model) && prefix[:len(model)] == model) {
				if p == 0 {
					return 0.10
				}
				return p
			}
		}
	}

	return ApiReferencePerM // fallback
}

// ── Worker response (OpenAI-compatible) ──────────────────────────────

type WorkerResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	Tier    string   `json:"tier"`
}

type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type ResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	TokenSource      string `json:"token_source"`
}

// ── Ledger ────────────────────────────────────────────────────────────

type LedgerEntry struct {
	Timestamp string  `json:"ts"`
	Event     string  `json:"event"`
	User      string  `json:"user"`
	Amount    float64 `json:"amount"`
	Detail    string  `json:"detail"`
}

// ── Status ────────────────────────────────────────────────────────────

type StatusResponse struct {
	Nodes              []*Node            `json:"nodes"`
	Balances           map[string]float64 `json:"balances"`
	TotalTasksComplete int                `json:"total_tasks_completed"`
	TotalTokensTraded  int64              `json:"total_tokens_traded"`
	TotalSaved         float64            `json:"total_saved"`    // $ saved vs API pricing
	TotalKMCost        float64            `json:"total_km_cost"`  // what users actually paid
	TotalApiCost       float64            `json:"total_api_cost"` // what API would have cost
}

type HealthResponse struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	NodesOnline   int    `json:"nodes_online"`
	NodesTotal    int    `json:"nodes_total"`
}
