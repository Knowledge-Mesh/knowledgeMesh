package network

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// EnvP2PPrometheusExport enables km_p2p_* Prometheus registration when set to 1, true, yes, or on.
const EnvP2PPrometheusExport = "KM_P2P_PROMETHEUS_EXPORT"

type P2PStats struct {
	TotalPeers  uint64
	DirectPeers uint64
	RelayPeers  uint64

	ConnectionFailures uint64

	HolePunchAttempts uint64
	HolePunchSuccess  uint64

	BytesSentDirect uint64
	BytesSentRelay  uint64

	MessagesSmall uint64
	MessagesLarge uint64
	MessagesDirect uint64
	MessagesRelay  uint64
}

func (s P2PStats) HolePunchSuccessRate() float64 {
	if s.HolePunchAttempts == 0 {
		return 0
	}
	return float64(s.HolePunchSuccess) / float64(s.HolePunchAttempts)
}

func (s P2PStats) MessageSizeLargePercent() float64 {
	total := s.MessagesSmall + s.MessagesLarge
	if total == 0 {
		return 0
	}
	return float64(s.MessagesLarge) * 100 / float64(total)
}

func (s P2PStats) MessageRouteRelayPercent() float64 {
	total := s.MessagesDirect + s.MessagesRelay
	if total == 0 {
		return 0
	}
	return float64(s.MessagesRelay) * 100 / float64(total)
}

type P2PObserver struct {
	totalPeers  atomic.Uint64
	directPeers atomic.Uint64
	relayPeers  atomic.Uint64

	connectionFailures atomic.Uint64

	holePunchAttempts atomic.Uint64
	holePunchSuccess  atomic.Uint64

	bytesSentDirect atomic.Uint64
	bytesSentRelay  atomic.Uint64

	messagesSmall  atomic.Uint64
	messagesLarge  atomic.Uint64
	messagesDirect atomic.Uint64
	messagesRelay  atomic.Uint64

	registerOnce sync.Once
}

func NewP2PObserver() *P2PObserver {
	return &P2PObserver{}
}

func (o *P2PObserver) ObserveConnectionSnapshot(total, direct, relay int) {
	o.totalPeers.Store(uint64(total))
	o.directPeers.Store(uint64(direct))
	o.relayPeers.Store(uint64(relay))
}

func (o *P2PObserver) ObserveConnectionFailure() { o.connectionFailures.Add(1) }
func (o *P2PObserver) ObserveHolePunchAttempt()  { o.holePunchAttempts.Add(1) }
func (o *P2PObserver) ObserveHolePunchSuccess()  { o.holePunchSuccess.Add(1) }

func (o *P2PObserver) ObserveTraffic(route string, bytes int) {
	if bytes <= 0 {
		return
	}
	switch route {
	case connTypeDirect:
		o.bytesSentDirect.Add(uint64(bytes))
	case connTypeRelay:
		o.bytesSentRelay.Add(uint64(bytes))
	}
}

func (o *P2PObserver) ObserveMessageSize(isLarge bool) {
	if isLarge {
		o.messagesLarge.Add(1)
		return
	}
	o.messagesSmall.Add(1)
}

func (o *P2PObserver) ObserveMessageRoute(route string) {
	switch route {
	case connTypeDirect:
		o.messagesDirect.Add(1)
	case connTypeRelay:
		o.messagesRelay.Add(1)
	}
}

func (o *P2PObserver) Snapshot() P2PStats {
	return P2PStats{
		TotalPeers:         o.totalPeers.Load(),
		DirectPeers:        o.directPeers.Load(),
		RelayPeers:         o.relayPeers.Load(),
		ConnectionFailures: o.connectionFailures.Load(),
		HolePunchAttempts:  o.holePunchAttempts.Load(),
		HolePunchSuccess:   o.holePunchSuccess.Load(),
		BytesSentDirect:    o.bytesSentDirect.Load(),
		BytesSentRelay:     o.bytesSentRelay.Load(),
		MessagesSmall:      o.messagesSmall.Load(),
		MessagesLarge:      o.messagesLarge.Load(),
		MessagesDirect:     o.messagesDirect.Load(),
		MessagesRelay:      o.messagesRelay.Load(),
	}
}

func (o *P2PObserver) RegisterPrometheus() {
	o.registerOnce.Do(func() {
		prometheus.MustRegister(
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "km_p2p_total_peers", Help: "Current total peers"}, func() float64 { return float64(o.totalPeers.Load()) }),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "km_p2p_direct_peers", Help: "Current peers with direct path"}, func() float64 { return float64(o.directPeers.Load()) }),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "km_p2p_relay_peers", Help: "Current peers on relay path"}, func() float64 { return float64(o.relayPeers.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_connection_failures_total", Help: "Total connection failures"}, func() float64 { return float64(o.connectionFailures.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_holepunch_attempts_total", Help: "Total hole punch attempts"}, func() float64 { return float64(o.holePunchAttempts.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_holepunch_success_total", Help: "Total hole punch successes"}, func() float64 { return float64(o.holePunchSuccess.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_bytes_sent_direct_total", Help: "Bytes sent on direct paths"}, func() float64 { return float64(o.bytesSentDirect.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_bytes_sent_relay_total", Help: "Bytes sent on relay paths"}, func() float64 { return float64(o.bytesSentRelay.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_messages_small_total", Help: "Small messages routed"}, func() float64 { return float64(o.messagesSmall.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_messages_large_total", Help: "Large messages routed"}, func() float64 { return float64(o.messagesLarge.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_messages_direct_total", Help: "Messages sent over direct route"}, func() float64 { return float64(o.messagesDirect.Load()) }),
			prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "km_p2p_messages_relay_total", Help: "Messages sent over relay route"}, func() float64 { return float64(o.messagesRelay.Load()) }),
		)
	})
}

var defaultP2PObserver = NewP2PObserver()

// ParsePrometheusExportEnv returns whether KM_P2P_PROMETHEUS_EXPORT requests metrics export (default off).
func ParsePrometheusExportEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(EnvP2PPrometheusExport)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// SetPrometheusExport registers km_p2p_* metrics with the default Prometheus registerer when enabled.
// Safe to call multiple times; registration happens at most once.
func SetPrometheusExport(enabled bool) {
	if !enabled {
		return
	}
	defaultP2PObserver.RegisterPrometheus()
}

// ApplyP2PMetricsExportForHost enables Prometheus export per HostConfig: non-nil EnableP2PPrometheusExport
// overrides the environment; otherwise KM_P2P_PROMETHEUS_EXPORT is used.
func ApplyP2PMetricsExportForHost(cfg HostConfig) {
	var on bool
	if cfg.EnableP2PPrometheusExport != nil {
		on = *cfg.EnableP2PPrometheusExport
	} else {
		on = ParsePrometheusExportEnv()
	}
	SetPrometheusExport(on)
}

// GetP2PStats returns an internal snapshot for decision-making and diagnostics.
func GetP2PStats() P2PStats { return defaultP2PObserver.Snapshot() }

// GetP2PObserver returns the shared observer for internal components.
func GetP2PObserver() *P2PObserver { return defaultP2PObserver }

