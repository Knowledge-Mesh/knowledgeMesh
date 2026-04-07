package network

import (
	"context"
	"log"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	host "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
)

// Default retry spacing: bounded jitter avoids synchronized retries across peers (less relay/UDP spam).
const (
	defaultRetryMin       = 10 * time.Second
	defaultRetryMax       = 20 * time.Second
	defaultMinGapAttempts = 2 * time.Second
	// OpportunisticSweepInterval periodically re-checks peers that still have relay paths after a direct path exists.
	defaultOpportunisticSweepInterval = 45 * time.Second
	// RelayCloseDelay gives in-flight streams a moment to finish before closing circuit connections after upgrade.
	defaultRelayCloseDelay = 200 * time.Millisecond
)

// HolePunchMetrics holds coarse counters for observability (not libp2p's internal DCUtR metrics).
type HolePunchMetrics struct {
	Attempts atomic.Uint64
	Successes atomic.Uint64
}

// SuccessRate returns successes/attempts, or 0 if no attempts.
func (m *HolePunchMetrics) SuccessRate() float64 {
	a := m.Attempts.Load()
	if a == 0 {
		return 0
	}
	return float64(m.Successes.Load()) / float64(a)
}

// HolePunchManagerConfig controls backoff for manual DCUtR retries on relay-only paths.
// Retries use uniform jitter in [RetryIntervalMin, RetryIntervalMax] plus a minimum gap between attempts.
type HolePunchManagerConfig struct {
	RetryIntervalMin time.Duration // default 10s
	RetryIntervalMax time.Duration // default 20s
	// MinGapBetweenAttempts enforces a floor between any two DirectConnect calls (limits churn).
	MinGapBetweenAttempts time.Duration // default 2s
	// OpportunisticSweepInterval runs a lightweight pass to DirectConnect relay-only peers and
	// to close leftover relay connections once a direct path exists. Zero uses default; negative disables.
	OpportunisticSweepInterval time.Duration
	// RelayCloseDelay waits after direct is confirmed before closing circuit connections (graceful transition).
	RelayCloseDelay time.Duration
}

func normalizeHolePunchConfig(c HolePunchManagerConfig) HolePunchManagerConfig {
	if c.RetryIntervalMin <= 0 {
		c.RetryIntervalMin = defaultRetryMin
	}
	if c.RetryIntervalMax <= 0 {
		c.RetryIntervalMax = defaultRetryMax
	}
	if c.RetryIntervalMax < c.RetryIntervalMin {
		c.RetryIntervalMax = c.RetryIntervalMin
	}
	if c.MinGapBetweenAttempts <= 0 {
		c.MinGapBetweenAttempts = defaultMinGapAttempts
	}
	if c.OpportunisticSweepInterval == 0 {
		c.OpportunisticSweepInterval = defaultOpportunisticSweepInterval
	}
	if c.RelayCloseDelay == 0 {
		c.RelayCloseDelay = defaultRelayCloseDelay
	}
	return c
}

// DefaultHolePunchManagerConfig returns production-safe defaults (10–20s jitter, 2s min gap).
func DefaultHolePunchManagerConfig() HolePunchManagerConfig {
	return HolePunchManagerConfig{
		RetryIntervalMin:           defaultRetryMin,
		RetryIntervalMax:           defaultRetryMax,
		MinGapBetweenAttempts:      defaultMinGapAttempts,
		OpportunisticSweepInterval: defaultOpportunisticSweepInterval,
		RelayCloseDelay:            defaultRelayCloseDelay,
	}
}

// HolePunchManager runs per-peer goroutines that call DCUtR DirectConnect when a peer is only
// reachable via relay, until a direct path exists or the peer disconnects. It also nudges retries
// when local addresses change (e.g. Wi‑Fi ↔ cellular IP change).
type HolePunchManager struct {
	h   host.Host
	hp  *holepunch.Service
	cfg HolePunchManagerConfig

	metrics HolePunchMetrics

	mu       sync.Mutex
	peers    map[peer.ID]*peerWorker // active worker per peer (pointer identity for safe cleanup)
	rootCtx  context.Context         // parent for workers (from Start)
	addrSub  event.Subscription
	notif    *netNotifee
	upgradeNotif *upgradeNotifee
	closeOnce sync.Once
	closed   bool

	upgradeMu sync.Mutex // per upgrade scheduling (avoid duplicate relay closes / punches)
}

// NewHolePunchManager creates a manager. If hp is nil (hole punching disabled), Start is a no-op.
func NewHolePunchManager(h host.Host, hp *holepunch.Service, cfg HolePunchManagerConfig) *HolePunchManager {
	cfg = normalizeHolePunchConfig(cfg)
	return &HolePunchManager{
		h:     h,
		hp:    hp,
		cfg:   cfg,
		peers: make(map[peer.ID]*peerWorker),
	}
}

type peerWorker struct {
	cancel context.CancelFunc
}

// Metrics returns the shared counter set (attempts, success rate).
func (m *HolePunchManager) Metrics() *HolePunchMetrics {
	return &m.metrics
}

// TriggerDirectConnect kicks off a best-effort direct-connect (DCUtR) attempt in
// the background for peer id. Intended for on-demand nudges from higher-level routing.
func (m *HolePunchManager) TriggerDirectConnect(id peer.ID) {
	if m == nil || m.hp == nil {
		return
	}
	go func() {
		m.metrics.Attempts.Add(1)
		GetP2PObserver().ObserveHolePunchAttempt()
		log.Printf("[holepunch] trigger peer=%s reason=message_router", id)
		if err := m.hp.DirectConnect(id); err != nil {
			log.Printf("[holepunch] trigger failed peer=%s: %v", id, err)
			return
		}
		m.metrics.Successes.Add(1)
		GetP2PObserver().ObserveHolePunchSuccess()
		log.Printf("[holepunch] trigger success peer=%s", id)
		m.scheduleRelayCloseAfterDirect(id, "message_router_trigger")
	}()
}

// HandleNetworkChange restarts hole punch retry workers for active relay-only peers.
func (m *HolePunchManager) HandleNetworkChange(reason string) {
	if m == nil {
		return
	}
	m.bumpPeers(reason)
}

// Start registers network and event-bus listeners and begins scheduling retries for relay-only peers.
func (m *HolePunchManager) Start(ctx context.Context) {
	if m.hp == nil {
		return
	}
	m.rootCtx = ctx

	sub, err := m.h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		log.Printf("[holepunch] subscribe EvtLocalAddressesUpdated: %v", err)
	} else {
		m.addrSub = sub
		go m.loopAddrEvents(ctx, sub)
	}

	n := &netNotifee{m: m}
	m.notif = n
	m.h.Network().Notify(n)

	un := &upgradeNotifee{m: m}
	m.upgradeNotif = un
	m.h.Network().Notify(un)

	if m.cfg.OpportunisticSweepInterval > 0 {
		go m.opportunisticSweepLoop(ctx)
	}
}

func (m *HolePunchManager) loopAddrEvents(ctx context.Context, sub event.Subscription) {
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			switch e.(type) {
			case *event.EvtLocalAddressesUpdated, event.EvtLocalAddressesUpdated:
				m.bumpPeers("local_addresses_updated")
			}
		}
	}
}

func (m *HolePunchManager) bumpPeers(reason string) {
	if m.rootCtx == nil {
		return
	}
	m.mu.Lock()
	for id, w := range m.peers {
		w.cancel()
		delete(m.peers, id)
	}
	m.mu.Unlock()

	// Restart workers for currently connected relay-only peers (new IP / NAT mapping may help DCUtR).
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.h.Network().Peers() {
		if relayOnlyToPeer(m.h, id) {
			m.startPeerLocked(m.rootCtx, id, reason)
		}
	}
}

func (m *HolePunchManager) startPeerLocked(parent context.Context, id peer.ID, reason string) {
	if m.closed {
		return
	}
	if _, ok := m.peers[id]; ok {
		return
	}
	if !relayOnlyToPeer(m.h, id) {
		return
	}
	peerCtx, cancel := context.WithCancel(parent)
	w := &peerWorker{cancel: cancel}
	m.peers[id] = w
	go m.runPeer(peerCtx, id, reason, w)
}

func (m *HolePunchManager) runPeer(ctx context.Context, id peer.ID, reason string, w *peerWorker) {
	defer func() {
		m.mu.Lock()
		if cur := m.peers[id]; cur == w {
			delete(m.peers, id)
		}
		m.mu.Unlock()
	}()

	log.Printf("[holepunch] start worker peer=%s trigger=%s", id, reason)

	var lastAttempt time.Time
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if !hasConnection(m.h, id) {
				log.Printf("[holepunch] peer=%s disconnected; stop retries", id)
				return
			}
			if hasDirectConnection(m.h, id) {
				if hasRelayConnection(m.h, id) {
					m.scheduleRelayCloseAfterDirect(id, "worker_direct_ready")
				}
				log.Printf("[holepunch] peer=%s already has direct path; stop retries", id)
				return
			}
			if !relayOnlyToPeer(m.h, id) {
				log.Printf("[holepunch] peer=%s no longer relay-only; stop retries", id)
				return
			}

			now := time.Now()
			if gap := m.cfg.MinGapBetweenAttempts - now.Sub(lastAttempt); gap > 0 {
				resetTimer(timer, gap)
				continue
			}

			lastAttempt = now
			m.metrics.Attempts.Add(1)
			GetP2PObserver().ObserveHolePunchAttempt()
			log.Printf("[holepunch] attempt peer=%s (relay-only path)", id)

			err := m.hp.DirectConnect(id)
			if err == nil {
				m.metrics.Successes.Add(1)
				GetP2PObserver().ObserveHolePunchSuccess()
				log.Printf("[holepunch] success peer=%s direct connection established", id)
				m.scheduleRelayCloseAfterDirect(id, "dcutr_ok")
				return
			}
			if hasDirectConnection(m.h, id) {
				m.metrics.Successes.Add(1)
				GetP2PObserver().ObserveHolePunchSuccess()
				log.Printf("[holepunch] success peer=%s direct connection detected after attempt", id)
				m.scheduleRelayCloseAfterDirect(id, "dcutr_detected")
				return
			}

			if err == holepunch.ErrHolePunchActive {
				log.Printf("[holepunch] peer=%s punch already active; will retry with backoff: %v", id, err)
			} else {
				log.Printf("[holepunch] upgrade failure peer=%s: %v", id, err)
			}

			d := jitterDuration(m.cfg.RetryIntervalMin, m.cfg.RetryIntervalMax)
			resetTimer(timer, d)
		}
	}
}

// Close stops all workers and unsubscribes.
func (m *HolePunchManager) Close() {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		for id, w := range m.peers {
			w.cancel()
			delete(m.peers, id)
		}
		m.mu.Unlock()
		if m.notif != nil {
			m.h.Network().StopNotify(m.notif)
		}
		if m.addrSub != nil {
			m.addrSub.Close()
		}
		if m.upgradeNotif != nil {
			m.h.Network().StopNotify(m.upgradeNotif)
		}
	})
}

type netNotifee struct {
	m *HolePunchManager
}

func (n *netNotifee) Connected(_ network.Network, conn network.Conn) {
	p := conn.RemotePeer()
	if !relayOnlyToPeer(n.m.h, p) {
		return
	}
	if n.m.rootCtx == nil {
		return
	}
	n.m.mu.Lock()
	defer n.m.mu.Unlock()
	n.m.startPeerLocked(n.m.rootCtx, p, "connected_relay_only")
}

func (n *netNotifee) Disconnected(_ network.Network, conn network.Conn) {
	p := conn.RemotePeer()
	n.m.mu.Lock()
	defer n.m.mu.Unlock()
	if w, ok := n.m.peers[p]; ok {
		w.cancel()
	}
}

func (n *netNotifee) Listen(network.Network, ma.Multiaddr)      {}
func (n *netNotifee) ListenClose(network.Network, ma.Multiaddr) {}

func hasConnection(h host.Host, id peer.ID) bool {
	return len(h.Network().ConnsToPeer(id)) > 0
}

// isRelayAddr reports whether the multiaddr uses a relay circuit (p2p-circuit).
func isRelayAddr(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// hasDirectConnection returns true if any open connection to id is not relay-only.
func hasDirectConnection(h host.Host, id peer.ID) bool {
	for _, c := range h.Network().ConnsToPeer(id) {
		if !isRelayAddr(c.RemoteMultiaddr()) {
			return true
		}
	}
	return false
}

// relayOnlyToPeer is true when we have at least one connection to id and all of them are relay paths.
func relayOnlyToPeer(h host.Host, id peer.ID) bool {
	conns := h.Network().ConnsToPeer(id)
	if len(conns) == 0 {
		return false
	}
	for _, c := range conns {
		if !isRelayAddr(c.RemoteMultiaddr()) {
			return false
		}
	}
	return true
}

func jitterDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	delta := max - min
	return min + time.Duration(rand.Uint64N(uint64(delta)+1))
}

// resetTimer clears a possibly stale channel read before Reset (Go timer semantics).
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}
