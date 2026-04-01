package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// secretsEqual performs a constant-time comparison of two secret strings
// to prevent timing side-channel attacks.
func secretsEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

type Handlers struct {
	registry     *Registry
	ledger       *Ledger
	escrow       *Escrow
	state        *PersistentState
	ghSync       *GitHubSync
	capacity     *CapacityTracker
	startAt      int64 // unix timestamp
	adminSecret  string
	brokerURL    string // public URL of this broker (for node-config responses)
	limiters     *RateLimiterMap
	emailHMACKey []byte // HMAC key for hashing emails (from KM_EMAIL_HMAC_KEY env var)
	registerMu   sync.Mutex // serializes registration to prevent race conditions
}

const maxMaxTokens = 16384 // maximum allowed max_tokens from buyer requests

// ── Rate Limiting ────────────────────────────────────────────────────

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiterMap struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
}

func NewRateLimiterMap() *RateLimiterMap {
	rl := &RateLimiterMap{
		limiters: make(map[string]*rateLimiterEntry),
	}
	// Background cleanup: evict entries not seen in the last 10 minutes
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiterMap) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for key, entry := range rl.limiters {
			if entry.lastSeen.Before(cutoff) {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiterMap) GetLimiter(key string, r rate.Limit, burst int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if entry, ok := rl.limiters[key]; ok {
		entry.lastSeen = time.Now()
		return entry.limiter
	}
	l := rate.NewLimiter(r, burst)
	rl.limiters[key] = &rateLimiterEntry{limiter: l, lastSeen: time.Now()}
	return l
}

func rateLimitByIP(rl *RateLimiterMap, r *http.Request, rateLimit rate.Limit, burst int) bool {
	ip := clientIP(r)
	limiter := rl.GetLimiter(ip, rateLimit, burst)
	return !limiter.Allow()
}

// clientIP extracts the client IP for rate limiting.
// Only trusts X-Forwarded-For when KM_TRUST_PROXY=true, and uses the
// second-to-last entry (the IP the trusted proxy saw connecting to it).
// If there's only one entry, use that directly.
// Otherwise falls back to r.RemoteAddr.
func clientIP(r *http.Request) string {
	if os.Getenv("KM_TRUST_PROXY") == "true" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) >= 2 {
				// Second-to-last = the IP the trusted proxy saw
				return strings.TrimSpace(parts[len(parts)-2])
			}
			// Single entry — the proxy added it directly
			return strings.TrimSpace(parts[0])
		}
	}
	// Strip port from RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// isAdminAuth checks whether the request carries the admin secret
// via the X-Admin-Secret header. Node secrets do NOT grant admin access.
func (h *Handlers) isAdminAuth(r *http.Request) bool {
	secret := r.Header.Get("X-Admin-Secret")
	return secret != "" && secretsEqual(secret, h.adminSecret)
}

// syncToGitHub triggers a background push if GitHub sync is enabled.
func (h *Handlers) syncToGitHub() {
	if h.ghSync != nil {
		h.ghSync.ForcePush()
	}
}

type NodeConfig struct {
	Name              string             `json:"name"`
	Tier              string             `json:"tier"`
	PricePerM         float64            `json:"price_per_million_tokens"`
	Models            map[string]float64 `json:"models,omitempty"` // model → price/M
	TokenBudget       int64              `json:"token_budget,omitempty"`
	BudgetWindowHours int                `json:"budget_window_hours,omitempty"`
	MaxConcurrent     int                `json:"max_concurrent,omitempty"`
	Activated         bool               `json:"activated"`
	RegisteredAt      int64              `json:"registered_at,omitempty"`
}

const maxRetries = 3

// sanitizeTokenCount validates worker-reported token counts against a
// server-side estimate derived from the actual request/response content.
// This prevents a malicious worker from inflating TotalTokens to drain
// the buyer's escrow. If the worker reports more than 3x the estimate
// the value is capped; if it reports 0, the estimate is used instead.
func sanitizeTokenCount(reported int, messages []Message, resp *WorkerResponse, taskID, handler string) int {
	// Estimate input tokens from message content
	inputChars := 0
	for _, m := range messages {
		inputChars += len(m.Content)
	}
	inputTokens := int(math.Ceil(float64(inputChars) / 3.8))

	// Estimate output tokens from response content
	outputChars := 0
	for _, c := range resp.Choices {
		outputChars += len(c.Message.Content)
	}
	outputTokens := int(math.Ceil(float64(outputChars) / 3.8))

	estimated := inputTokens + outputTokens
	if estimated < 1 {
		estimated = 1
	}

	const maxMultiple = 3

	if reported <= 0 {
		log.Printf("[%s] %s: WARNING worker reported 0 tokens, using estimate %d", handler, taskID, estimated)
		return estimated
	}

	cap := estimated * maxMultiple
	if reported > cap {
		log.Printf("[%s] %s: WARNING worker reported %d tokens but estimate is %d — capping at %d (3x estimate)",
			handler, taskID, reported, estimated, cap)
		return cap
	}

	return reported
}

// ── Helpers ──────────────────────────────────────────────────────────

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	// Allow localhost and 127.0.0.1 on any port
	if strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1") {
		return true
	}
	// Allow *.onrender.com
	if strings.HasSuffix(origin, ".onrender.com") &&
		(strings.HasPrefix(origin, "https://") || strings.HasPrefix(origin, "http://")) {
		return true
	}
	return false
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Secret")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
