package config

type NodeConfig struct {
	APIAddr string
	P2PAddr string
}

func DefaultNodeConfig() NodeConfig {
	return NodeConfig{
		APIAddr: ":8080",
		P2PAddr: "/ip4/0.0.0.0/udp/0/quic-v1",
	}
}
