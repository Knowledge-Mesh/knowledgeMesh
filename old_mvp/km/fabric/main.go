package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

func now() time.Time {
	return time.Now()
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	ledgerPath := os.Getenv("KM_LEDGER_PATH")
	if ledgerPath == "" {
		ledgerPath = "ledger.txt"
	}

	// State file lives next to the ledger
	statePath := ledgerPath + ".state.json"

	// GitHub sync — pulls remote state and ledger BEFORE we load them into memory.
	// On Render, local files are ephemeral — this restores state from the last push.
	ghSync := NewGitHubSync(ledgerPath, statePath)
	if ghSync != nil {
		ghSync.Start()
	} else {
		log.Println("[github-sync] Disabled — set KM_GITHUB_TOKEN and KM_GITHUB_REPO to enable")
	}

	// Initialize components AFTER sync has restored files from GitHub
	registry := NewRegistry()
	ledger := NewLedger(ledgerPath)
	escrow := NewEscrow()
	state := NewPersistentState(statePath)

	// Optionally seed users (for testing). In production, users register via the web dashboard.
	if defaultUsers := os.Getenv("KM_USERS"); defaultUsers != "" {
		for _, name := range splitUsers(defaultUsers) {
			ledger.EnsureUser(name, 100.0)
		}
	}

	adminSecret := os.Getenv("KM_ADMIN_SECRET")
	if adminSecret == "" {
		log.Println("[auth] FATAL: KM_ADMIN_SECRET is not set. Refusing to start with default secret.")
		log.Println("[auth] Set KM_ADMIN_SECRET environment variable to a strong secret.")
		os.Exit(1)
	}

	brokerURL := os.Getenv("KM_BROKER_URL")
	if brokerURL == "" {
		brokerURL = fmt.Sprintf("https://km-broker.onrender.com")
	}

	// Load HMAC key for email hashing (used to salt email hashes)
	emailHMACKey := os.Getenv("KM_EMAIL_HMAC_KEY")
	if emailHMACKey == "" {
		emailHMACKey = "km-default-hmac-key-change-me"
		log.Println("[auth] WARNING: KM_EMAIL_HMAC_KEY not set — using default key. Set this env var in production.")
	}

	h := &Handlers{
		registry:     registry,
		ledger:       ledger,
		escrow:       escrow,
		state:        state,
		ghSync:       ghSync,
		capacity:     NewCapacityTracker(),
		startAt:      time.Now().Unix(),
		adminSecret:  adminSecret,
		brokerURL:    brokerURL,
		limiters:     NewRateLimiterMap(),
		emailHMACKey: []byte(emailHMACKey),
	}

	// Start heartbeat reaper
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go registry.StartReaper(ctx)

	// Periodic cleanup of stale pending (never-activated) nodes — runs every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			state.mu.Lock()
			now := time.Now().Unix()
			var stale []string
			for name, cfg := range state.NodeConfigs {
				if !cfg.Activated && cfg.RegisteredAt > 0 && now-cfg.RegisteredAt > 86400 {
					stale = append(stale, name)
				}
			}
			for _, name := range stale {
				delete(state.NodeConfigs, name)
				delete(state.NodeSecrets, name)
				delete(state.UserEmails, name)
				log.Printf("[cleanup] Removed stale pending node '%s' (registered >24h ago, never activated)", name)
			}
			if len(stale) > 0 {
				state.saveUnlocked()
			}
			state.mu.Unlock()
		}
	}()

	// Periodic liveness check — probes online nodes every 30 seconds
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			nodes := registry.OnlineNodes()
			for _, n := range nodes {
				healthURL := n.TunnelURL + "/health"
				resp, err := client.Get(healthURL)
				if err != nil {
					registry.MarkSuspect(n.ID)
					log.Printf("[liveness] Node '%s' (%s) failed health check: %v", n.Name, n.ID, err)
					continue
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					registry.MarkSuspect(n.ID)
					log.Printf("[liveness] Node '%s' (%s) returned status %d", n.Name, n.ID, resp.StatusCode)
				}
			}
		}
	}()

	// Periodic capacity record pruning — removes token records older than 48 hours
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.capacity.PruneOldRecords(48 * time.Hour)
		}
	}()

	// Routes
	mux := http.NewServeMux()

	// Web dashboard
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := webFS.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// API endpoints
	mux.HandleFunc("GET /health", corsMiddleware(h.Health))
	mux.HandleFunc("GET /status", corsMiddleware(h.Status))
	mux.HandleFunc("GET /models", corsMiddleware(h.Models))
	mux.HandleFunc("GET /ledger", corsMiddleware(h.LedgerView))
	mux.HandleFunc("POST /register", corsMiddleware(h.Register))
	mux.HandleFunc("POST /register-user", corsMiddleware(h.RegisterUser))
	mux.HandleFunc("POST /admin/invites", corsMiddleware(h.CreateInvite))
	mux.HandleFunc("GET /admin/invites", corsMiddleware(h.ListInvites))
	mux.HandleFunc("POST /admin/reset", corsMiddleware(h.AdminReset))
	mux.HandleFunc("GET /admin/reset-tokens", corsMiddleware(h.ListResetTokens))
	mux.HandleFunc("POST /update-price", corsMiddleware(h.UpdatePrice))
	mux.HandleFunc("POST /deregister", corsMiddleware(h.Deregister))
	mux.HandleFunc("GET /node-config", corsMiddleware(h.NodeConfigHandler))
	mux.HandleFunc("POST /heartbeat", corsMiddleware(h.Heartbeat))
	mux.HandleFunc("POST /task", corsMiddleware(h.Task))
	mux.HandleFunc("POST /v1/chat/completions", corsMiddleware(h.OpenAIChat))
	mux.HandleFunc("POST /update-tier", corsMiddleware(h.UpdateTier))
	mux.HandleFunc("POST /update-limits", corsMiddleware(h.UpdateLimits))
	mux.HandleFunc("GET /whoami", corsMiddleware(h.WhoAmI))
	mux.HandleFunc("POST /recover", corsMiddleware(h.Recover))
	mux.HandleFunc("POST /reset-secret", corsMiddleware(h.ResetSecret))

	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("[fabric] KnowledgeMesh broker starting on %s", addr)
	log.Printf("[fabric] Ledger: %s", ledgerPath)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[fabric] Server error: %v", err)
	}
}

func splitUsers(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
