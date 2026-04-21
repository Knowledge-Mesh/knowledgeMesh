package network

import (
	"strings"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

type HolePunchResultInfo struct {
	Time    time.Time `json:"time"`
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Source  string    `json:"source,omitempty"`
}

type PeerConnDetail struct {
	RemoteMultiaddr string `json:"remoteMultiaddr"`
	DirectPath      bool   `json:"directPath"`
}

type PeerDebugReport struct {
	LocalPeerID          string               `json:"localPeerId"`
	PeerID               string               `json:"peerId"`
	Connected            bool                 `json:"connected"`
	ConnectionType       string               `json:"connectionType"`
	TaggedConnectionType string               `json:"taggedConnectionType,omitempty"`
	NATReachability      string               `json:"natReachability,omitempty"`
	Connections          []PeerConnDetail     `json:"connections"`
	LastHolePunch        *HolePunchResultInfo `json:"lastHolePunch,omitempty"`
}

func BuildPeerDebugReport(h host.Host, tracker *ConnectionTypeTracker, hp *HolePunchManager, id peer.ID) PeerDebugReport {
	rep := PeerDebugReport{
		LocalPeerID:     h.ID().String(),
		PeerID:          id.String(),
		NATReachability: CurrentNATReachability(),
	}
	if tracker != nil {
		rep.TaggedConnectionType = tracker.TaggedConnectionType(id)
	}
	conns := h.Network().ConnsToPeer(id)
	rep.Connected = len(conns) > 0
	for _, c := range conns {
		rep.Connections = append(rep.Connections, PeerConnDetail{
			RemoteMultiaddr: c.RemoteMultiaddr().String(),
			DirectPath:      !isRelayAddr(c.RemoteMultiaddr()),
		})
	}
	rep.ConnectionType = classifyConnectionType(h, id)
	if hp != nil {
		if last, ok := hp.LastHolePunchResult(id); ok {
			rep.LastHolePunch = &last
		}
	}
	return rep
}

func ParsePeerIDFromArg(s string) (peer.ID, error) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "/"))
	return peer.Decode(s)
}
