package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const charsPerToken = 3.8

// EstimateTokens mirrors the Rust worker's estimation heuristic.
func EstimateTokens(text string) int {
	return int(math.Ceil(float64(len(text)) / charsPerToken))
}

// NodePrice returns the effective price for a node given a model request.
// If the node has per-model pricing and the model matches, use that price.
// Otherwise fall back to the node's flat price.
func NodePrice(n *Node, requestedModel string) float64 {
	if requestedModel != "" && len(n.Models) > 0 {
		// Exact match
		if price, ok := n.Models[requestedModel]; ok {
			return price
		}
		// Prefix match: "claude" matches "claude-sonnet-4-*", "gpt-4o" matches "gpt-4o-*"
		for model, price := range n.Models {
			if strings.HasPrefix(model, requestedModel) {
				return price
			}
		}
	}
	return n.PricePerM
}

// NodeSupportsModel checks if a node can serve the requested model.
// If no model requested, any node matches.
// If model requested, node must have it in its Models map (exact or prefix match).
func NodeSupportsModel(n *Node, requestedModel string) bool {
	if requestedModel == "" {
		return true // any model is fine
	}
	if len(n.Models) == 0 {
		return true // legacy node without model list — allow (best effort)
	}
	// Exact match
	if _, ok := n.Models[requestedModel]; ok {
		return true
	}
	// Case-insensitive prefix match: "claude" matches "claude-sonnet-4-*", "gpt-4o" matches "gpt-4o-*"
	lower := strings.ToLower(requestedModel)
	for model := range n.Models {
		if strings.HasPrefix(strings.ToLower(model), lower) {
			return true
		}
	}
	return false
}

// PickNodes returns all eligible nodes sorted by price (cheapest first),
// plus the escrow amount based on the cheapest node.
// Supports model-based filtering: if req.Model is set, only nodes offering
// that model are eligible, and pricing uses the per-model price.
// If ct is non-nil, also filters by concurrency limits and token budgets.
func PickNodes(nodes []*Node, req TaskRequest, ct *CapacityTracker) ([]*Node, float64, error) {
	// Sum up all message content for token estimation
	totalChars := 0
	for _, m := range req.Messages {
		totalChars += len(m.Content)
	}

	// Estimate input tokens from message content
	inputTokens := int(math.Ceil(float64(totalChars) / charsPerToken))

	// Estimate output tokens: use max_tokens if buyer specified it, otherwise assume output ≈ input
	outputEstimate := inputTokens
	if req.MaxTokens > 0 {
		outputEstimate = req.MaxTokens
	}

	estimatedTotal := inputTokens + outputEstimate
	if estimatedTotal < 100 {
		estimatedTotal = 100 // minimum estimate
	}

	requestedModel := req.Model

	// Filter eligible nodes
	var eligible []*Node
	skippedForBudget := 0
	for _, n := range nodes {
		// Must be online
		if n.Status != "online" {
			continue
		}

		// Must be activated (worker has connected at least once)
		if !n.Activated {
			continue
		}

		// Node pinning: if buyer requested a specific node, only match that one
		if req.Node != "" && n.Name != req.Node {
			continue
		}

		// Model filter
		if !NodeSupportsModel(n, requestedModel) {
			continue
		}

		// Tier preference
		if req.TierPreference != "" && n.Tier != req.TierPreference {
			continue
		}

		// Budget check using effective price for the requested model
		effectivePrice := NodePrice(n, requestedModel)
		if req.MaxBudget > 0 {
			estimatedCost := float64(estimatedTotal) / 1_000_000.0 * effectivePrice
			if estimatedCost > req.MaxBudget {
				continue
			}
		}

		// Capacity checks (concurrency + token budget)
		if ct != nil {
			// Concurrency limit
			maxConc := EffectiveMaxConcurrent(n.Tier, n.MaxConcurrent)
			if maxConc > 0 && ct.InflightCount(n.ID) >= maxConc {
				continue
			}

			// Token budget check: ensure the node's remaining budget can handle
			// the estimated total tokens (input + output) for this request
			if n.TokenBudget > 0 {
				remaining := ct.RemainingBudget(n.ID, n.TokenBudget, n.BudgetWindowHours)
				if remaining >= 0 && remaining < int64(estimatedTotal) {
					skippedForBudget++
					continue
				}
			}
		}

		eligible = append(eligible, n)
	}

	if len(eligible) == 0 {
		if skippedForBudget > 0 {
			return nil, 0, fmt.Errorf("no worker with sufficient token budget for this request (~%d tokens estimated)", estimatedTotal)
		}
		if req.Node != "" && requestedModel != "" {
			return nil, 0, fmt.Errorf("node '%s' is not available or does not support model '%s'", req.Node, requestedModel)
		}
		if req.Node != "" {
			return nil, 0, fmt.Errorf("node '%s' is not available. It may be offline", req.Node)
		}
		if requestedModel != "" {
			return nil, 0, fmt.Errorf("no worker available for model '%s'. Use GET /models to see available models", requestedModel)
		}
		return nil, 0, fmt.Errorf("no matching worker available. There may be no online nodes, or your budget is too low")
	}

	// Sort by effective price for the requested model (cheapest first)
	sort.Slice(eligible, func(i, j int) bool {
		return NodePrice(eligible[i], requestedModel) < NodePrice(eligible[j], requestedModel)
	})

	// Escrow based on cheapest node
	cheapestPrice := NodePrice(eligible[0], requestedModel)

	// If buyer specified max_tokens, use that for accurate escrow instead of heuristic
	escrowTokens := estimatedTotal
	if req.MaxTokens > 0 {
		escrowTokens = req.MaxTokens
	}
	escrowAmount := float64(escrowTokens) / 1_000_000.0 * cheapestPrice
	// Add 20% buffer (only for heuristic estimates)
	if req.MaxTokens <= 0 {
		escrowAmount *= 1.2
	}

	return eligible, escrowAmount, nil
}
