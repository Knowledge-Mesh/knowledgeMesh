package network

import (
	"context"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
)

const DefaultQUICListenAddr = "/ip4/0.0.0.0/udp/0/quic-v1"

func NewHost(_ context.Context, listenAddr string) (host.Host, error) {
	return libp2p.New(
		libp2p.ListenAddrStrings(listenAddr),
	)
}
