package network

import (
	"context"
	"log"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func hasRelayConnection(h host.Host, id peer.ID) bool {
	for _, c := range h.Network().ConnsToPeer(id) {
		if isRelayAddr(c.RemoteMultiaddr()) {
			return true
		}
	}
	return false
}

// closeRelayConnectionsToPeer closes circuit-relay connections to id. Call only when a direct path exists
// so new streams prefer direct (avoids duplicate sends on relay + direct).
func closeRelayConnectionsToPeer(h host.Host, id peer.ID) int {
	var n int
	for _, c := range h.Network().ConnsToPeer(id) {
		if !isRelayAddr(c.RemoteMultiaddr()) {
			continue
		}
		addr := c.RemoteMultiaddr().String()
		if err := c.Close(); err != nil {
			log.Printf("[upgrade] relay close failed peer=%s addr=%s: %v", id, addr, err)
			continue
		}
		n++
		log.Printf("[upgrade] closed relay transport peer=%s addr=%s", id, addr)
	}
	return n
}

func (m *HolePunchManager) scheduleRelayCloseAfterDirect(id peer.ID, reason string) {
	if m == nil || m.h == nil {
		return
	}
	delay := m.cfg.RelayCloseDelay
	if delay <= 0 {
		delay = defaultRelayCloseDelay
	}
	go func() {
		time.Sleep(delay)
		m.upgradeMu.Lock()
		defer m.upgradeMu.Unlock()

		if !hasDirectConnection(m.h, id) {
			log.Printf("[upgrade] skipped relay prune peer=%s reason=%s (no direct path)", id, reason)
			return
		}
		if !hasRelayConnection(m.h, id) {
			return
		}
		n := closeRelayConnectionsToPeer(m.h, id)
		if n > 0 {
			log.Printf("[upgrade] relay→direct peer=%s reason=%s pruned_relay_conns=%d", id, reason, n)
		}
	}()
}

func (m *HolePunchManager) opportunisticSweepLoop(ctx context.Context) {
	interval := m.cfg.OpportunisticSweepInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runOpportunisticSweep()
		}
	}
}

func (m *HolePunchManager) runOpportunisticSweep() {
	if m == nil || m.hp == nil || m.closed {
		return
	}
	m.mu.Lock()
	root := m.rootCtx
	m.mu.Unlock()
	if root == nil {
		return
	}

	for _, id := range m.h.Network().Peers() {
		if hasDirectConnection(m.h, id) && hasRelayConnection(m.h, id) {
			m.scheduleRelayCloseAfterDirect(id, "periodic_sweep")
			continue
		}
		if relayOnlyToPeer(m.h, id) {
			m.mu.Lock()
			if _, exists := m.peers[id]; !exists {
				m.startPeerLocked(root, id, "opportunistic_sweep")
			}
			m.mu.Unlock()
		}
	}
}

// Notifee hook: when a new connection arrives and we already have direct + relay, prune relay after delay.
type upgradeNotifee struct {
	m *HolePunchManager
}

func (n *upgradeNotifee) Connected(net network.Network, conn network.Conn) {
	if n.m == nil {
		return
	}
	id := conn.RemotePeer()
	if hasDirectConnection(n.m.h, id) && hasRelayConnection(n.m.h, id) {
		n.m.scheduleRelayCloseAfterDirect(id, "conn_notify_mixed_paths")
	}
}

func (n *upgradeNotifee) Disconnected(network.Network, network.Conn) {}
func (n *upgradeNotifee) Listen(network.Network, ma.Multiaddr)      {}
func (n *upgradeNotifee) ListenClose(network.Network, ma.Multiaddr) {}
