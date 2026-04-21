package network

// defaultRelayAddrs is the built-in static relay/bootstrap candidate set used when callers do not
// provide relays via env/CLI. Keep these as full multiaddrs with /p2p/<peerID>.
var defaultRelayAddrs = []string{
	"/dns4/relay.p2pinfer.cloud/tcp/4001/p2p/12D3KooWCyQ1Tug72SEpoCEf2daaAXa9AZgHXochzHWCcxEvVca8",
	"/dns4/relay.p2pinfer.cloud/udp/4001/quic-v1/p2p/12D3KooWCyQ1Tug72SEpoCEf2daaAXa9AZgHXochzHWCcxEvVca8",
}

// defaultStaticRelayAddrs returns the built-in default relay set.
func defaultStaticRelayAddrs() []string {
	out := make([]string, len(defaultRelayAddrs))
	copy(out, defaultRelayAddrs)
	return out
}
