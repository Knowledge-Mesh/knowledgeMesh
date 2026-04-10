package network

import (
	"context"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"

	ma "github.com/multiformats/go-multiaddr"
)

// reachableHost is implemented by libp2p's closableBasicHost / closableRoutedHost (they embed *basichost.BasicHost).
// There is no host.AutoNAT() on core host.Host; AutoNAT v2 state is surfaced this way.
type reachableHost interface {
	Reachability() network.Reachability
	ConfirmedAddrs() (reachable, unreachable, unknown []ma.Multiaddr)
}

type legacyAutoNATHost interface {
	GetAutoNat() autonat.AutoNAT
}

// StartSellerReachabilityLogger logs AutoNAT-related state at seller startup and again after a short delay
// so dial-back probes (scheduled by libp2p after BasicHost.Start) have time to run.
func StartSellerReachabilityLogger(ctx context.Context, h host.Host) {
	rh, ok := h.(reachableHost)
	if !ok {
		log.Printf("[p2p] reachability: host type %T has no Reachability/ConfirmedAddrs (expected libp2p BasicHost wrapper)", h)
		return
	}

	logOnce := func(when string) {
		rch := rh.Reachability()
		reachable, unreachable, unknown := rh.ConfirmedAddrs()
		log.Printf("[p2p] autonat %s: reachability=%s confirmed reachable=%d unreachable=%d unknown=%d",
			when, rch.String(), len(reachable), len(unreachable), len(unknown))
	}
	if lh, ok := h.(legacyAutoNATHost); ok {
		if an := lh.GetAutoNat(); an != nil {
			log.Printf("[p2p] autonat legacy Status=%s", an.Status().String())
		}
	}
	logOnce("startup")
	go func() {
		t := time.NewTimer(3 * time.Second)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			logOnce("after_probe_window")
		}
	}()
}
