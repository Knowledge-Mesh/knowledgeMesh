package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var validNodeName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

const starterCredits = 10.0

func (h *Handlers) RegisterUser(w http.ResponseWriter, r *http.Request) {
	if rateLimitByIP(h.limiters, r, 1, 3) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests. Try again in a few seconds."})
		return
	}

	// Serialize registrations to prevent race conditions (check-then-act on
	// name uniqueness, invite codes, etc.)
	h.registerMu.Lock()
	defer h.registerMu.Unlock()

	var req struct {
		Name              string  `json:"name"`
		InviteCode        string  `json:"invite_code"`
		Tier              string  `json:"tier"`
		PricePerM         float64 `json:"price_per_million_tokens"`
		Email             string  `json:"email"`
		TokenBudget       int64   `json:"token_budget,omitempty"`
		BudgetWindowHours int     `json:"budget_window_hours,omitempty"`
		MaxConcurrent     int     `json:"max_concurrent,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 100*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if !validNodeName.MatchString(req.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be 1-32 characters, alphanumeric, hyphens, or underscores only"})
		return
	}

	// Check if user already exists
	if h.ledger.UserExists(req.Name) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "name '" + req.Name + "' is already taken. Pick a different name.",
		})
		return
	}

	// Validate invite code
	if req.InviteCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invite code is required"})
		return
	}
	if !h.state.GetInviteCode(req.InviteCode) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid invite code"})
		return
	}

	// Email is required for account recovery
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required (used for account recovery)"})
		return
	}

	// Default tier and price
	if req.Tier == "" {
		req.Tier = "subscription"
	}
	if req.PricePerM <= 0 {
		req.PricePerM = 0.50
	}

	// Create user with starter credits
	if err := h.ledger.EnsureUser(req.Name, starterCredits); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Generate node secret and atomically persist everything in one write
	nodeSecret := fmt.Sprintf("km-sec-%s", uuid.New().String()[:12])
	h.state.RegisterNode(req.Name, nodeSecret, h.hashEmail(req.Email), req.InviteCode, &NodeConfig{
		Name:              req.Name,
		Tier:              req.Tier,
		PricePerM:         req.PricePerM,
		TokenBudget:       req.TokenBudget,
		BudgetWindowHours: req.BudgetWindowHours,
		MaxConcurrent:     req.MaxConcurrent,
		Activated:         false,
		RegisteredAt:      time.Now().Unix(),
	})

	h.syncToGitHub()

	log.Printf("[auth] User '%s' registered — tier=%s price=%.2f/M", req.Name, req.Tier, req.PricePerM)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":        req.Name,
		"credits":     starterCredits,
		"node_secret": nodeSecret,
		"tier":        req.Tier,
		"price":       req.PricePerM,
		"status":      "registered",
	})
}

// Node config endpoint — worker fetches its config using node secret
func (h *Handlers) NodeConfigHandler(w http.ResponseWriter, r *http.Request) {
	// Accept secret from Authorization header (preferred) or query param (backward compat)
	secret := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		secret = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if secret == "" {
		secret = r.URL.Query().Get("secret")
	}
	if secret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Authorization Bearer header or secret parameter is required"})
		return
	}

	// Find which node this secret belongs to
	nodeName := h.state.SecretForName(secret)
	if nodeName == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid node secret"})
		return
	}

	config, hasConfig := h.state.GetNodeConfig(nodeName)
	if !hasConfig {
		// User registered but no config stored — use defaults
		config = NodeConfig{
			Name:      nodeName,
			Tier:      "subscription",
			PricePerM: 0.50,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":                     config.Name,
		"tier":                     config.Tier,
		"price_per_million_tokens": config.PricePerM,
		"broker_url":               h.brokerURL,
		"node_secret":              secret,
	})
}

// ── WhoAmI ───────────────────────────────────────────────────────────
// Pass your secret, get back your identity. No more "what's my node name?"
func (h *Handlers) WhoAmI(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	if secret == "" {
		// Also check Authorization header
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			secret = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if secret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "secret parameter or Authorization Bearer header is required"})
		return
	}

	// Find which node this secret belongs to
	nodeName := h.state.SecretForName(secret)
	if nodeName == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid secret"})
		return
	}

	bal, _ := h.ledger.Balance(nodeName)
	config, hasConfig := h.state.GetNodeConfig(nodeName)

	resp := map[string]interface{}{
		"name":    nodeName,
		"credits": bal,
	}

	if hasConfig {
		resp["tier"] = config.Tier
		resp["price_per_million_tokens"] = config.PricePerM
	}

	// Check if email is registered (don't reveal the actual email, just that it exists)
	if _, hasEmail := h.state.GetUserEmail(nodeName); hasEmail {
		resp["email_registered"] = true
	} else {
		resp["email_registered"] = false
	}

	// Check if node is online
	nodes := h.registry.AllNodes()
	for _, n := range nodes {
		if n.Name == nodeName {
			resp["node_status"] = n.Status
			resp["node_id"] = n.ID
			break
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ── Account Recovery ─────────────────────────────────────────────────

// Recover initiates account recovery — generates a reset token if email matches.
func (h *Handlers) Recover(w http.ResponseWriter, r *http.Request) {
	if rateLimitByIP(h.limiters, r, 0.2, 2) { // 1 per 5 seconds
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many recovery attempts. Try again later."})
		return
	}

	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and email are required"})
		return
	}

	// Check if user exists
	if !h.ledger.UserExists(req.Name) {
		// Don't reveal whether the user exists — always return same response
		writeJSON(w, http.StatusOK, map[string]string{"message": "if the email matches, a reset token has been generated. Contact admin with the token."})
		return
	}

	// Check if email matches
	storedHash, hasEmail := h.state.GetUserEmail(req.Name)
	if !hasEmail || storedHash != h.hashEmail(req.Email) {
		// Same response to prevent enumeration
		writeJSON(w, http.StatusOK, map[string]string{"message": "if the email matches, a reset token has been generated. Contact admin with the token."})
		return
	}

	// Generate reset token (valid for 1 hour)
	token := fmt.Sprintf("km-reset-%s", uuid.New().String()[:12])
	h.state.SetResetToken(token, &ResetToken{
		Name:      req.Name,
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	})
	h.syncToGitHub()

	log.Printf("[auth] Recovery token generated for '%s'", req.Name)

	// Send reset token via email (Resend)
	emailSent := sendRecoveryEmail(req.Email, req.Name, token, h.brokerURL)

	if emailSent {
		// Don't expose the token in the response — it was sent to email
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "recovery email sent. Check your inbox for the reset token, then POST /reset-secret.",
		})
	} else {
		// Email not configured or failed — NEVER return the token in the response.
		// Admin can retrieve pending tokens via GET /admin/reset-tokens.
		log.Printf("[auth] Email not sent for '%s' — token stored for admin retrieval via GET /admin/reset-tokens", req.Name)
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "account recovery requires email to be configured. Contact the network admin.",
		})
	}
}

// ResetSecret validates a reset token and issues a new node secret.
func (h *Handlers) ResetSecret(w http.ResponseWriter, r *http.Request) {
	if rateLimitByIP(h.limiters, r, 0.5, 3) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts."})
		return
	}

	var req struct {
		ResetToken string `json:"reset_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ResetToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reset_token is required"})
		return
	}

	// Look up and validate token
	rt, exists := h.state.GetResetToken(req.ResetToken)
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired reset token"})
		return
	}

	if time.Now().Unix() > rt.ExpiresAt {
		h.state.DeleteResetToken(req.ResetToken)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reset token has expired. Request a new one via POST /recover."})
		return
	}

	// Generate new secret
	newSecret := fmt.Sprintf("km-sec-%s", uuid.New().String()[:12])
	h.state.SetNodeSecret(rt.Name, newSecret)

	// Burn the reset token
	h.state.DeleteResetToken(req.ResetToken)
	h.syncToGitHub()

	log.Printf("[auth] Secret reset for '%s'", rt.Name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":        rt.Name,
		"node_secret": newSecret,
		"message":     "secret reset successfully. Update KM_NODE_SECRET on your worker.",
	})
}

// hashEmail creates an HMAC-SHA256 hash of the email (lowercased, trimmed)
// using a server-side key for salt. This prevents rainbow-table attacks on
// the stored hashes. The key comes from KM_EMAIL_HMAC_KEY env var.
func (h *Handlers) hashEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	mac := hmac.New(sha256.New, h.emailHMACKey)
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}
