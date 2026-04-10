package network

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
)

func StartP2PDebugHTTPServerIfConfigured(ctx context.Context, httpAddr string, h host.Host, ct *ConnectionTypeTracker, hp *HolePunchManager) {
	addr := strings.TrimSpace(httpAddr)
	if addr == "" {
		addr = ParseP2PDebugHTTPAddr()
	}
	if addr == "" || !P2PDebug() {
		return
	}
	_ = ServeP2PDebugHTTP(ctx, addr, h, ct, hp)
}

// NewP2PDebugHTTPMux returns the P2P debug HTTP routes (peer report, reachability).
// Handlers return 503 when P2PDebug() is false.
func NewP2PDebugHTTPMux(h host.Host, ct *ConnectionTypeTracker, hp *HolePunchManager) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/p2p/peer/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !P2PDebug() {
			http.Error(w, "p2p debug disabled", http.StatusServiceUnavailable)
			return
		}
		raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/debug/p2p/peer/"), "/")
		id, err := ParsePeerIDFromArg(raw)
		if err != nil {
			http.Error(w, "invalid peer id", http.StatusBadRequest)
			return
		}
		rep := BuildPeerDebugReport(h, ct, hp, id)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	})
	mux.HandleFunc("/debug/p2p/reachability", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !P2PDebug() {
			http.Error(w, "p2p debug disabled", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"natReachability": CurrentNATReachability(),
		})
	})
	return mux
}

func ServeP2PDebugHTTP(ctx context.Context, addr string, h host.Host, ct *ConnectionTypeTracker, hp *HolePunchManager) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           NewP2PDebugHTTPMux(h, ct, hp),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("[p2p-debug] http listening on %s", addr)
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[p2p-debug] http server: %v", err)
		}
	}()
	return nil
}
