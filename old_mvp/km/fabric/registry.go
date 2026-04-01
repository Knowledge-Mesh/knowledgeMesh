package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const heartbeatTimeout = 30 * time.Second

type Registry struct {
	mu    sync.RWMutex
	nodes map[string]*Node // keyed by node_id
}

func NewRegistry() *Registry {
	return &Registry{
		nodes: make(map[string]*Node),
	}
}

func (r *Registry) Register(req RegisterRequest) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if a node with the same name already exists — update it
	for _, n := range r.nodes {
		if n.Name == req.Name {
			n.Tier = req.Tier
			n.PricePerM = req.PricePerM
			n.Models = req.Models
			n.TunnelURL = req.TunnelURL
			n.LastHeartbeat = time.Now()
			n.Status = "online"
			n.TokenBudget = req.TokenBudget
			n.BudgetWindowHours = req.BudgetWindowHours
			n.MaxConcurrent = req.MaxConcurrent
			log.Printf("[registry] Re-registered node %s (%s) models=%v", n.Name, n.ID, modelNames(n.Models))
			return n, nil
		}
	}

	// New node
	node := &Node{
		ID:                fmt.Sprintf("node-%s", uuid.New().String()[:8]),
		Name:              req.Name,
		Tier:              req.Tier,
		PricePerM:         req.PricePerM,
		Models:            req.Models,
		TunnelURL:         req.TunnelURL,
		LastHeartbeat:     time.Now(),
		Status:            "online",
		TokenBudget:       req.TokenBudget,
		BudgetWindowHours: req.BudgetWindowHours,
		MaxConcurrent:     req.MaxConcurrent,
	}
	r.nodes[node.ID] = node
	log.Printf("[registry] Registered new node %s (%s) tier=%s models=%v", node.Name, node.ID, node.Tier, modelNames(node.Models))
	return node, nil
}

func modelNames(models map[string]float64) []string {
	var names []string
	for name := range models {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Heartbeat(nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[nodeID]
	if !ok {
		return fmt.Errorf("unknown node %s", nodeID)
	}
	node.LastHeartbeat = time.Now()
	node.Status = "online"
	return nil
}

func (r *Registry) OnlineNodes() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Node
	for _, n := range r.nodes {
		if n.Status == "online" {
			result = append(result, n)
		}
	}
	return result
}

func (r *Registry) AllNodes() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Node
	for _, n := range r.nodes {
		result = append(result, n)
	}
	return result
}

// VisibleNodes returns nodes whose last heartbeat is within the last 5 minutes.
// Nodes that have been unresponsive longer than that are hidden entirely.
func (r *Registry) VisibleNodes() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	const staleThreshold = 5 * time.Minute
	var result []*Node
	for _, n := range r.nodes {
		if time.Since(n.LastHeartbeat) <= staleThreshold {
			copy := *n
			result = append(result, &copy)
		}
	}
	return result
}

func (r *Registry) Get(nodeID string) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[nodeID]
}

// Remove deletes a node from the registry (used when health check fails).
func (r *Registry) Remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
}

// RemoveByName deletes a node by name (used for deregistration).
func (r *Registry) RemoveByName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, n := range r.nodes {
		if n.Name == name {
			delete(r.nodes, id)
			log.Printf("[registry] Removed node %s (%s)", name, id)
			return
		}
	}
}

// MarkSuspect marks a node as suspect after a failed task forward.
// The node stays in the registry but is deprioritized. The next heartbeat
// will restore it to online if the worker is still alive.
func (r *Registry) MarkSuspect(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		node.Status = "suspect"
		log.Printf("[registry] Node %s (%s) marked suspect — task forward failed", node.Name, node.ID)
	}
}

// StartReaper marks nodes as offline if they miss heartbeats.
func (r *Registry) UpdatePrice(name string, pricePerM float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, node := range r.nodes {
		if node.Name == name {
			node.PricePerM = pricePerM
			return true
		}
	}
	return false
}

func (r *Registry) UpdateLimits(name string, tokenBudget int64, budgetWindowHours int, maxConcurrent int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, node := range r.nodes {
		if node.Name == name {
			node.TokenBudget = tokenBudget
			node.BudgetWindowHours = budgetWindowHours
			node.MaxConcurrent = maxConcurrent
			return true
		}
	}
	return false
}

func (r *Registry) UpdateTier(name string, tier string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, node := range r.nodes {
		if node.Name == name {
			node.Tier = tier
			return true
		}
	}
	return false
}

func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes = make(map[string]*Node)
	log.Printf("[registry] Reset — all nodes cleared")
}

func (r *Registry) StartReaper(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			for id, n := range r.nodes {
				if n.Status == "online" && time.Since(n.LastHeartbeat) > heartbeatTimeout {
					n.Status = "offline"
					log.Printf("[registry] Node %s (%s) marked offline — missed heartbeat", n.Name, n.ID)
				}
				// Remove nodes offline for more than 1 hour
				if n.Status == "offline" && time.Since(n.LastHeartbeat) > 1*time.Hour {
					delete(r.nodes, id)
					log.Printf("[registry] Removed stale node %s (%s) — offline > 1h", n.Name, id)
				}
			}
			r.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
