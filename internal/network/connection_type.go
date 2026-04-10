package network

import (
	"context"
	"sync"

	host "github.com/libp2p/go-libp2p/core/host"
	coreNetwork "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	connTypeDirect = "direct"
	connTypeRelay  = "relay"

	connTagDirect = "conn:direct"
	connTagRelay  = "conn:relay"
)

// ConnectionTypeTracker classifies peer connectivity (direct vs relay), logs new
// connections, and tags peers via libp2p connmgr for later routing decisions.
type ConnectionTypeTracker struct {
	h host.Host

	mu   sync.RWMutex
	tags map[peer.ID]string

	notifee *connectionTypeNotifee
}

func NewConnectionTypeTracker(h host.Host) *ConnectionTypeTracker {
	return &ConnectionTypeTracker{
		h:    h,
		tags: make(map[peer.ID]string),
	}
}

func (t *ConnectionTypeTracker) Start() {
	n := &connectionTypeNotifee{tracker: t}
	t.notifee = n
	t.h.Network().Notify(n)
}

func (t *ConnectionTypeTracker) Close() {
	if t.notifee != nil {
		t.h.Network().StopNotify(t.notifee)
	}
}

// HandleNetworkChange refreshes connection-type tags for all currently connected peers
// and asks connmgr to re-trim based on updated path quality.
func (t *ConnectionTypeTracker) HandleNetworkChange() {
	for _, id := range t.h.Network().Peers() {
		t.updateTag(id)
	}
	t.h.ConnManager().TrimOpenConns(context.Background())
}

// IsDirectConnection reports whether peerID currently has at least one non-relay
// connection (e.g. QUIC/TCP direct path).
func (t *ConnectionTypeTracker) IsDirectConnection(peerID peer.ID) bool {
	for _, c := range t.h.Network().ConnsToPeer(peerID) {
		if !isRelayMultiaddr(c.RemoteMultiaddr()) {
			return true
		}
	}
	return false
}

// IsRelayConnection reports whether peerID currently has only relay connections.
func (t *ConnectionTypeTracker) IsRelayConnection(peerID peer.ID) bool {
	conns := t.h.Network().ConnsToPeer(peerID)
	if len(conns) == 0 {
		return false
	}
	for _, c := range conns {
		if !isRelayMultiaddr(c.RemoteMultiaddr()) {
			return false
		}
	}
	return true
}

// TaggedConnectionType returns the last tagged connection type for peerID.
func (t *ConnectionTypeTracker) TaggedConnectionType(peerID peer.ID) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tags[peerID]
}

func (t *ConnectionTypeTracker) classify(peerID peer.ID) string {
	switch {
	case t.IsDirectConnection(peerID):
		return connTypeDirect
	case t.IsRelayConnection(peerID):
		return connTypeRelay
	default:
		return ""
	}
}

func (t *ConnectionTypeTracker) updateTag(peerID peer.ID) {
	typ := t.classify(peerID)

	t.mu.Lock()
	if typ == "" {
		delete(t.tags, peerID)
	} else {
		t.tags[peerID] = typ
	}
	t.mu.Unlock()

	cm := t.h.ConnManager()
	cm.UntagPeer(peerID, connTagDirect)
	cm.UntagPeer(peerID, connTagRelay)

	switch typ {
	case connTypeDirect:
		cm.TagPeer(peerID, connTagDirect, 100)
	case connTypeRelay:
		cm.TagPeer(peerID, connTagRelay, 50)
	}

	t.refreshSnapshotMetrics()
}


func (t *ConnectionTypeTracker) refreshSnapshotMetrics() {
	total := 0
	direct := 0
	relay := 0
	for _, id := range t.h.Network().Peers() {
		total++
		if t.IsDirectConnection(id) {
			direct++
		} else if t.IsRelayConnection(id) {
			relay++
		}
	}
	GetP2PObserver().ObserveConnectionSnapshot(total, direct, relay)
}
func isRelayMultiaddr(addr ma.Multiaddr) bool {
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

type connectionTypeNotifee struct {
	tracker *ConnectionTypeTracker
}

func (n *connectionTypeNotifee) Connected(_ coreNetwork.Network, conn coreNetwork.Conn) {
	peerID := conn.RemotePeer()
	n.tracker.updateTag(peerID)
	if n.tracker.IsDirectConnection(peerID) {
		P2PDebugLog("connected peer=%s type=direct remote=%s", peerID, conn.RemoteMultiaddr())
		return
	}
	if n.tracker.IsRelayConnection(peerID) {
		P2PDebugLog("connected peer=%s type=relay remote=%s", peerID, conn.RemoteMultiaddr())
		return
	}
	P2PDebugLog("connected peer=%s type=unknown remote=%s", peerID, conn.RemoteMultiaddr())
}

func (n *connectionTypeNotifee) Disconnected(_ coreNetwork.Network, conn coreNetwork.Conn) {
	n.tracker.updateTag(conn.RemotePeer())
}

func (n *connectionTypeNotifee) Listen(coreNetwork.Network, ma.Multiaddr)      {}
func (n *connectionTypeNotifee) ListenClose(coreNetwork.Network, ma.Multiaddr) {}
