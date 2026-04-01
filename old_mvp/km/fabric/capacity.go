package main

import (
	"math"
	"sync"
	"time"
)

// TokenRecord represents a timestamped token usage event.
type TokenRecord struct {
	Tokens    int64
	Timestamp time.Time
}

// CapacityTracker tracks token usage and inflight requests per node.
type CapacityTracker struct {
	mu       sync.RWMutex
	usage    map[string][]TokenRecord // node_id -> timestamped records
	inflight map[string]int           // node_id -> current inflight count
}

// NewCapacityTracker creates a new CapacityTracker.
func NewCapacityTracker() *CapacityTracker {
	return &CapacityTracker{
		usage:    make(map[string][]TokenRecord),
		inflight: make(map[string]int),
	}
}

// AddInflight increments the inflight count for a node.
func (ct *CapacityTracker) AddInflight(nodeID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.inflight[nodeID]++
}

// RemoveInflight decrements the inflight count for a node.
func (ct *CapacityTracker) RemoveInflight(nodeID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.inflight[nodeID]--
	if ct.inflight[nodeID] < 0 {
		ct.inflight[nodeID] = 0
	}
}

// InflightCount returns the current inflight count for a node.
func (ct *CapacityTracker) InflightCount(nodeID string) int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.inflight[nodeID]
}

// RecordTokens appends a timestamped token usage record for a node.
func (ct *CapacityTracker) RecordTokens(nodeID string, tokens int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.usage[nodeID] = append(ct.usage[nodeID], TokenRecord{
		Tokens:    tokens,
		Timestamp: time.Now(),
	})
}

// TokensUsedInWindow sums tokens used in the last windowHours hours for a node.
// Also prunes records older than the window.
// If windowHours <= 0, returns 0 (unlimited).
func (ct *CapacityTracker) TokensUsedInWindow(nodeID string, windowHours int) int64 {
	if windowHours <= 0 {
		return 0
	}

	ct.mu.RLock()
	defer ct.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)
	var total int64
	for _, r := range ct.usage[nodeID] {
		if r.Timestamp.After(cutoff) {
			total += r.Tokens
		}
	}
	return total
}

// PruneOldRecords removes expired token records. Call from a background goroutine.
func (ct *CapacityTracker) PruneOldRecords(maxAge time.Duration) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for nodeID, records := range ct.usage {
		var kept []TokenRecord
		for _, r := range records {
			if r.Timestamp.After(cutoff) {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(ct.usage, nodeID)
		} else {
			ct.usage[nodeID] = kept
		}
	}
}

// availableTokens returns how many tokens are available in the current window.
// Returns math.MaxInt64 if budget is 0 (unlimited).
func (ct *CapacityTracker) availableTokens(nodeID string, budget int64, windowHours int) int64 {
	if budget <= 0 {
		return math.MaxInt64
	}
	used := ct.TokensUsedInWindow(nodeID, windowHours)
	avail := budget - used
	if avail < 0 {
		return 0
	}
	return avail
}

// RemainingBudget returns how many tokens the node has left in its current
// budget window. Returns -1 if the node has no budget (unlimited).
func (ct *CapacityTracker) RemainingBudget(nodeID string, budget int64, windowHours int) int64 {
	if budget <= 0 {
		return -1 // unlimited
	}
	return ct.availableTokens(nodeID, budget, windowHours)
}

// EffectiveMaxConcurrent returns the effective max concurrent value.
// If maxConcurrent > 0, uses that. Otherwise uses tier defaults.
// Tier defaults: subscription=1, api=2, openai=2, ollama/local=0 (unlimited).
func EffectiveMaxConcurrent(tier string, maxConcurrent int) int {
	if maxConcurrent > 0 {
		return maxConcurrent
	}
	switch tier {
	case "subscription":
		return 1
	case "api":
		return 2
	case "openai":
		return 2
	default: // ollama, local, etc.
		return 0 // unlimited
	}
}
