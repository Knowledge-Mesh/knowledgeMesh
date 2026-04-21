package network

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
	coreNetwork "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
)

const (
	defaultMessageThresholdBytes = 3200 * 1024
	defaultDirectWaitTimeout     = 3 * time.Second
)

type MessageRouterConfig struct {
	// LargeMessageThresholdBytes controls when direct-preference routing kicks in.
	LargeMessageThresholdBytes int
	// DirectWaitTimeout controls how long to wait for a direct path after triggering hole punching.
	DirectWaitTimeout time.Duration
}

func DefaultMessageRouterConfig() MessageRouterConfig {
	return MessageRouterConfig{
		LargeMessageThresholdBytes: defaultMessageThresholdBytes,
		DirectWaitTimeout:          defaultDirectWaitTimeout,
	}
}

func normalizeMessageRouterConfig(cfg MessageRouterConfig) MessageRouterConfig {
	if cfg.LargeMessageThresholdBytes <= 0 {
		cfg.LargeMessageThresholdBytes = defaultMessageThresholdBytes
	}
	if cfg.DirectWaitTimeout <= 0 {
		cfg.DirectWaitTimeout = defaultDirectWaitTimeout
	}
	return cfg
}

type MessageRouterMetrics struct {
	TotalSent  atomic.Uint64
	DirectSent atomic.Uint64
	RelaySent  atomic.Uint64
}

func (m *MessageRouterMetrics) DirectPercent() float64 {
	total := m.TotalSent.Load()
	if total == 0 {
		return 0
	}
	return float64(m.DirectSent.Load()) * 100 / float64(total)
}

func (m *MessageRouterMetrics) RelayPercent() float64 {
	total := m.TotalSent.Load()
	if total == 0 {
		return 0
	}
	return float64(m.RelaySent.Load()) * 100 / float64(total)
}

// MessageRouter routes sends by payload size: small payloads use any available
// path, large payloads prefer direct and can trigger hole punching before relay fallback.
type MessageRouter struct {
	h         host.Host
	tracker   *ConnectionTypeTracker
	holepunch *HolePunchManager
	cfg       MessageRouterConfig
	metrics   MessageRouterMetrics
}

func NewMessageRouter(h host.Host, tracker *ConnectionTypeTracker, hp *HolePunchManager, cfg MessageRouterConfig) *MessageRouter {
	return &MessageRouter{
		h:         h,
		tracker:   tracker,
		holepunch: hp,
		cfg:       normalizeMessageRouterConfig(cfg),
	}
}

func (r *MessageRouter) Metrics() *MessageRouterMetrics {
	return &r.metrics
}

func (r *MessageRouter) SendRequest(ctx context.Context, target peer.ID, pid protocol.ID, request []byte) ([]byte, error) {
	msgSize := len(request)

	// Small messages: prefer direct when available so traffic moves off relay after upgrade (no duplicate path use).
	if msgSize < r.cfg.LargeMessageThresholdBytes {
		dialCtx := ctx
		if r.isDirect(target) {
			dialCtx = coreNetwork.WithForceDirectDial(ctx, "message-router-small-prefer-direct")
		}
		resp, route, err := sendRequestWithRoute(dialCtx, r.h, target, pid, request)
		r.recordRoute(msgSize, route)
		return resp, err
	}

	// Large messages: prefer direct path when available.
	if r.isDirect(target) {
		directCtx := coreNetwork.WithForceDirectDial(ctx, "message-router-large-prefer-direct")
		resp, route, err := sendRequestWithRoute(directCtx, r.h, target, pid, request)
		r.recordRoute(msgSize, route)
		return resp, err
	}

	// No direct path yet: trigger hole punching and wait briefly for direct connectivity.
	if r.holepunch != nil {
		r.holepunch.TriggerDirectConnect(target)
	}
	waitCtx, cancel := context.WithTimeout(ctx, r.cfg.DirectWaitTimeout)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if r.isDirect(target) {
			directCtx := coreNetwork.WithForceDirectDial(ctx, "message-router-direct-ready")
			resp, route, err := sendRequestWithRoute(directCtx, r.h, target, pid, request)
			r.recordRoute(msgSize, route)
			return resp, err
		}
		select {
		case <-waitCtx.Done():
			resp, route, err := sendRequestWithRoute(ctx, r.h, target, pid, request)
			r.recordRoute(msgSize, route)
			return resp, err
		case <-tick.C:
		}
	}
}

func (r *MessageRouter) isDirect(target peer.ID) bool {
	if r.tracker != nil {
		return r.tracker.IsDirectConnection(target)
	}
	return hasDirectConnection(r.h, target)
}

func (r *MessageRouter) recordRoute(msgSize int, route string) {
	r.metrics.TotalSent.Add(1)
	GetP2PObserver().ObserveMessageSize(msgSize >= r.cfg.LargeMessageThresholdBytes)
	GetP2PObserver().ObserveMessageRoute(route)
	switch route {
	case connTypeDirect:
		r.metrics.DirectSent.Add(1)
	case connTypeRelay:
		r.metrics.RelaySent.Add(1)
	}
	log.Printf("[router] message_size=%d route=%s direct_pct=%.2f relay_pct=%.2f",
		msgSize, route, r.metrics.DirectPercent(), r.metrics.RelayPercent())
}
