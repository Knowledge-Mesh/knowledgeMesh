package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	nodes := h.registry.VisibleNodes()
	online := 0
	for _, n := range nodes {
		if n.Status == "online" {
			online++
		}
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:        "ok",
		UptimeSeconds: now().Unix() - h.startAt,
		NodesOnline:   online,
		NodesTotal:    len(nodes),
	})
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	nodes := h.registry.VisibleNodes()
	tokens, _ := h.ledger.TokenStats()

	// Count online nodes
	online := 0
	for _, n := range nodes {
		if n.Status == "online" {
			online++
		}
	}

	_, kmCost := h.ledger.TokenStats()
	apiCost := float64(tokens) * ApiReferencePerM / 1_000_000

	// Enrich nodes with capacity info (public-safe — no secrets exposed)
	type NodeWithCapacity struct {
		*Node
		Inflight       int   `json:"inflight,omitempty"`
		TokensUsed     int64 `json:"tokens_used_in_window,omitempty"`
		AvailTokens    int64 `json:"available_tokens,omitempty"`
		EffectiveMaxCC int   `json:"effective_max_concurrent,omitempty"`
	}

	enriched := make([]NodeWithCapacity, 0, len(nodes))
	for _, n := range nodes {
		// Cross-reference activation status from persistent state
		if cfg, ok := h.state.GetNodeConfig(n.Name); ok {
			n.Activated = cfg.Activated
		}
		nwc := NodeWithCapacity{Node: n}
		nwc.Inflight = h.capacity.InflightCount(n.ID)
		nwc.EffectiveMaxCC = EffectiveMaxConcurrent(n.Tier, n.MaxConcurrent)
		if n.TokenBudget > 0 {
			nwc.TokensUsed = h.capacity.TokensUsedInWindow(n.ID, n.BudgetWindowHours)
			nwc.AvailTokens = n.TokenBudget - nwc.TokensUsed
			if nwc.AvailTokens < 0 {
				nwc.AvailTokens = 0
			}
		}
		enriched = append(enriched, nwc)
	}

	// Pending nodes are not shown on the dashboard — they clutter the UI.
	// They will appear once their worker connects and activates them.

	resp := map[string]interface{}{
		"nodes":                 enriched,
		"total_tasks_completed": h.ledger.TaskCount(),
		"total_tokens_traded":   tokens,
		"total_km_cost":         kmCost,
		"total_api_cost":        apiCost,
		"total_saved":           apiCost - kmCost,
	}

	// Admin gets balances too
	if h.isAdminAuth(r) {
		balances, _ := h.ledger.Balances()
		resp["balances"] = balances
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	if rateLimitByIP(h.limiters, r, 1, 5) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests."})
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 100*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.TunnelURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and tunnel_url are required"})
		return
	}

	// Validate tunnel_url to prevent SSRF via attacker-controlled URLs
	if err := validateTunnelURL(req.TunnelURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid tunnel_url: %v", err)})
		return
	}

	// Node name must be registered via web dashboard with invite code first
	if !h.ledger.UserExists(req.Name) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("node '%s' not registered. Go to the dashboard and register with an invite code first.", req.Name),
		})
		return
	}

	// Validate node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Name)
	if hasSecret && !secretsEqual(req.NodeSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid node secret. Use the secret shown when you registered on the dashboard.",
		})
		return
	}

	// Apply stored limits from config if the request doesn't specify them
	if config, ok := h.state.GetNodeConfig(req.Name); ok {
		if req.TokenBudget == 0 && config.TokenBudget > 0 {
			req.TokenBudget = config.TokenBudget
		}
		if req.BudgetWindowHours == 0 && config.BudgetWindowHours > 0 {
			req.BudgetWindowHours = config.BudgetWindowHours
		}
		if req.MaxConcurrent == 0 && config.MaxConcurrent > 0 {
			req.MaxConcurrent = config.MaxConcurrent
		}
	}

	node, err := h.registry.Register(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Activate the node on first worker connection
	if cfg, ok := h.state.GetNodeConfig(req.Name); ok && !cfg.Activated {
		h.state.UpdateNodeConfig(req.Name, func(c *NodeConfig) {
			c.Activated = true
		})
		h.syncToGitHub()
		log.Printf("[registry] Node '%s' activated (first worker connection)", req.Name)
	}

	// Set Activated on the in-memory registry node (read-only check)
	if cfg, ok := h.state.GetNodeConfig(req.Name); ok {
		node.Activated = cfg.Activated
	}

	// Note: health check removed from broker side — new Cloudflare tunnel DNS
	// takes too long to propagate to Render's resolver. Instead, the worker
	// self-tests locally before registering. The broker's retry/failover
	// handles any tunnel connectivity issues on real tasks.

	writeJSON(w, http.StatusOK, RegisterResponse{
		NodeID:  node.ID,
		Message: "registered",
	})
}

func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Authenticate: look up the node and validate its secret
	node := h.registry.Get(req.NodeID)
	if node == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown node %s", req.NodeID)})
		return
	}
	expectedSecret, hasSecret := h.state.GetNodeSecret(node.Name)
	if hasSecret {
		if req.NodeSecret == "" || !secretsEqual(req.NodeSecret, expectedSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing node_secret"})
			return
		}
	}

	if err := h.registry.Heartbeat(req.NodeID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Deregister removes a node from the active registry (goes offline).
// Credentials and config are preserved — node can restart and re-register.
func (h *Handlers) Deregister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		NodeSecret string `json:"node_secret"`
		Permanent  bool   `json:"permanent,omitempty"` // if true, delete everything
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.NodeSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and node_secret are required"})
		return
	}

	// Validate node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Name)
	if !hasSecret || !secretsEqual(req.NodeSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid node secret"})
		return
	}

	// Remove from registry (stops receiving tasks)
	h.registry.RemoveByName(req.Name)

	if req.Permanent {
		// Permanent: remove config and secret so they can re-register later with a new invite
		h.state.DeleteNodeSecret(req.Name)
		h.state.DeleteNodeConfig(req.Name)
		h.state.DeleteUserEmail(req.Name)
		h.syncToGitHub()
		log.Printf("[registry] Node '%s' permanently deregistered", req.Name)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":    req.Name,
			"message": "node permanently removed. Use a new invite code to rejoin.",
		})
	} else {
		// Graceful: just go offline, can restart and re-register
		log.Printf("[registry] Node '%s' went offline (graceful shutdown)", req.Name)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":    req.Name,
			"message": "node offline. Restart worker to rejoin.",
		})
	}
}

// Update node price (requires node secret)
func (h *Handlers) UpdatePrice(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	var req struct {
		Name       string  `json:"name"`
		NodeSecret string  `json:"node_secret"`
		PricePerM  float64 `json:"price_per_million_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.NodeSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and node_secret are required"})
		return
	}

	// Validate node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Name)
	if !hasSecret || !secretsEqual(req.NodeSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid node secret"})
		return
	}

	if req.PricePerM < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "price must be >= 0"})
		return
	}

	// Update price in registry and config
	h.registry.UpdatePrice(req.Name, req.PricePerM)
	h.state.UpdateNodeConfig(req.Name, func(cfg *NodeConfig) {
		cfg.PricePerM = req.PricePerM
	})
	h.syncToGitHub()

	log.Printf("[registry] Price updated for %s: $%.2f/M", req.Name, req.PricePerM)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    req.Name,
		"price":   req.PricePerM,
		"message": "price updated",
	})
}

// Update node tier (requires node secret)
func (h *Handlers) UpdateTier(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	var req struct {
		Name       string `json:"name"`
		NodeSecret string `json:"node_secret"`
		Tier       string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.NodeSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and node_secret are required"})
		return
	}

	if req.Tier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tier is required"})
		return
	}

	// Validate node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Name)
	if !hasSecret || !secretsEqual(req.NodeSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid node secret"})
		return
	}

	// Update tier in registry and config
	h.registry.UpdateTier(req.Name, req.Tier)
	h.state.UpdateNodeConfig(req.Name, func(cfg *NodeConfig) {
		cfg.Tier = req.Tier
	})
	h.syncToGitHub()

	log.Printf("[registry] Tier updated for %s: %s", req.Name, req.Tier)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    req.Name,
		"tier":    req.Tier,
		"message": "tier updated",
	})
}

// UpdateLimits updates token budget, window, and concurrency limits for a node.
func (h *Handlers) UpdateLimits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string `json:"name"`
		NodeSecret        string `json:"node_secret"`
		TokenBudget       *int64 `json:"token_budget,omitempty"`
		BudgetWindowHours *int   `json:"budget_window_hours,omitempty"`
		MaxConcurrent     *int   `json:"max_concurrent,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.NodeSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and node_secret are required"})
		return
	}

	// Validate node secret
	expectedSecret, hasSecret := h.state.GetNodeSecret(req.Name)
	if !hasSecret || !secretsEqual(req.NodeSecret, expectedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid node secret"})
		return
	}

	// Update config — if it doesn't exist yet, create one
	config, ok := h.state.GetNodeConfig(req.Name)
	if !ok {
		config = NodeConfig{Name: req.Name}
	}

	if req.TokenBudget != nil {
		config.TokenBudget = *req.TokenBudget
	}
	if req.BudgetWindowHours != nil {
		config.BudgetWindowHours = *req.BudgetWindowHours
	}
	if req.MaxConcurrent != nil {
		config.MaxConcurrent = *req.MaxConcurrent
	}

	// Update live node in registry
	h.registry.UpdateLimits(req.Name, config.TokenBudget, config.BudgetWindowHours, config.MaxConcurrent)

	h.state.SetNodeConfig(req.Name, &config)
	h.syncToGitHub()

	log.Printf("[registry] Limits updated for %s: budget=%d window=%dh concurrent=%d",
		req.Name, config.TokenBudget, config.BudgetWindowHours, config.MaxConcurrent)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":                req.Name,
		"token_budget":        config.TokenBudget,
		"budget_window_hours": config.BudgetWindowHours,
		"max_concurrent":      config.MaxConcurrent,
		"message":             "limits updated",
	})
}

// ── Models ───────────────────────────────────────────────────────────
// Lists all models available across the mesh, with the best (cheapest) price
// and how many nodes offer each model.
func (h *Handlers) Models(w http.ResponseWriter, r *http.Request) {
	nodes := h.registry.OnlineNodes()

	type ModelInfo struct {
		Model     string   `json:"model"`
		BestPrice float64  `json:"best_price_per_million_tokens"`
		NodeCount int      `json:"node_count"`
		Tiers     []string `json:"tiers"`
	}

	modelMap := make(map[string]*ModelInfo)

	for _, n := range nodes {
		if len(n.Models) == 0 {
			// Legacy node without model list — skip or show as "unknown"
			continue
		}
		for model, price := range n.Models {
			info, exists := modelMap[model]
			if !exists {
				info = &ModelInfo{
					Model:     model,
					BestPrice: price,
					NodeCount: 0,
				}
				modelMap[model] = info
			}
			info.NodeCount++
			if price < info.BestPrice {
				info.BestPrice = price
			}
			// Track unique tiers
			tierFound := false
			for _, t := range info.Tiers {
				if t == n.Tier {
					tierFound = true
					break
				}
			}
			if !tierFound {
				info.Tiers = append(info.Tiers, n.Tier)
			}
		}
	}

	var models []ModelInfo
	for _, info := range modelMap {
		models = append(models, *info)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": models,
		"count":  len(models),
	})
}

func (h *Handlers) LedgerView(w http.ResponseWriter, r *http.Request) {
	recent := h.ledger.RecentEntries(50)

	resp := map[string]interface{}{
		"recent":     recent,
		"task_count": h.ledger.TaskCount(),
	}

	// Balances are sensitive — only show to authenticated users
	if h.isAdminAuth(r) {
		balances, _ := h.ledger.Balances()
		resp["balances"] = balances
	}

	writeJSON(w, http.StatusOK, resp)
}
