package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// Admin: generate invite codes
func (h *Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	// Check admin secret
	secret := r.Header.Get("X-Admin-Secret")
	if secret == "" || !secretsEqual(secret, h.adminSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized. Include X-Admin-Secret header."})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Count == 0 {
		req.Count = 1
	}
	if req.Count > 20 {
		req.Count = 20
	}

	codes := make([]string, req.Count)
	for i := 0; i < req.Count; i++ {
		code := fmt.Sprintf("km-%s", uuid.New().String()[:8])
		h.state.SetInviteCode(code)
		codes[i] = code
	}

	h.syncToGitHub()

	log.Printf("[auth] Generated %d invite codes", req.Count)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codes": codes,
	})
}

// Admin: list active invite codes
func (h *Handlers) ListInvites(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-Admin-Secret")
	if secret == "" || !secretsEqual(secret, h.adminSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized. Include X-Admin-Secret header."})
		return
	}

	codes := h.state.AllInviteCodes()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codes": codes,
		"count": len(codes),
	})
}

// Admin: list pending reset tokens (for manual recovery when email is not configured)
func (h *Handlers) ListResetTokens(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-Admin-Secret")
	if secret == "" || !secretsEqual(secret, h.adminSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized. Include X-Admin-Secret header."})
		return
	}

	tokens := h.state.AllResetTokens()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reset_tokens": tokens,
		"count":        len(tokens),
	})
}

// Admin: reset all state (ledger, nodes, invites)
func (h *Handlers) AdminReset(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-Admin-Secret")
	if secret == "" || !secretsEqual(secret, h.adminSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized. Include X-Admin-Secret header."})
		return
	}

	h.registry.Reset()
	h.ledger.Reset()
	h.escrow.Reset()

	// Clear persistent state
	h.state.Reset()
	h.syncToGitHub()

	log.Printf("[admin] Full state reset")
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset complete"})
}
