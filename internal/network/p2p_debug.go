package network

import (
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/event"
	host "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// EnvP2PDebug enables verbose P2P debug logging when set to 1, true, yes, or on.
const EnvP2PDebug = "KM_P2P_DEBUG"

// EnvP2PDebugHTTP is an optional HTTP listen address (example: 127.0.0.1:9091)
// used by StartP2PDebugHTTPServerIfConfigured.
const EnvP2PDebugHTTP = "KM_P2P_DEBUG_HTTP"

var p2pDebugEnabled atomic.Bool
var lastReachability atomic.Value // string

func init() {
	lastReachability.Store("")
}

func ParseP2PDebugEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(EnvP2PDebug)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ParseP2PDebugHTTPAddr() string {
	return strings.TrimSpace(os.Getenv(EnvP2PDebugHTTP))
}

func SetP2PDebug(enabled bool) {
	p2pDebugEnabled.Store(enabled)
}

func P2PDebug() bool {
	return p2pDebugEnabled.Load()
}

func P2PDebugLog(format string, v ...any) {
	if !P2PDebug() {
		return
	}
	log.Printf("[p2p-debug] "+format, v...)
}

func ApplyP2PDebugForHost(cfg HostConfig) {
	on := ParseP2PDebugEnv()
	if cfg.EnableP2PDebug != nil {
		on = *cfg.EnableP2PDebug
	}
	SetP2PDebug(on)
}

func CurrentNATReachability() string {
	s, _ := lastReachability.Load().(string)
	return s
}

// StartP2PDebugMonitors subscribes to reachability updates and logs them in debug mode.
func StartP2PDebugMonitors(ctx context.Context, h host.Host) {
	if !P2PDebug() || h == nil {
		return
	}
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged))
	if err != nil {
		log.Printf("[p2p-debug] subscribe EvtLocalReachabilityChanged: %v", err)
		return
	}
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-sub.Out():
				if !ok {
					return
				}
				ev, ok := e.(event.EvtLocalReachabilityChanged)
				if !ok {
					continue
				}
				lastReachability.Store(ev.Reachability.String())
				P2PDebugLog("nat reachability=%s", ev.Reachability.String())
			}
		}
	}()
}

func classifyConnectionType(h host.Host, id peer.ID) string {
	conns := h.Network().ConnsToPeer(id)
	if len(conns) == 0 {
		return "disconnected"
	}
	if hasDirectConnection(h, id) {
		return connTypeDirect
	}
	if relayOnlyToPeer(h, id) {
		return connTypeRelay
	}
	return "mixed"
}
