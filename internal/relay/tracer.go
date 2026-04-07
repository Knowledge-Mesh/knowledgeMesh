package relay

import (
	"log"
	"time"

	pbv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/pb"
)

// loggingRelayTracer provides basic relay service visibility.
type loggingRelayTracer struct{}

func (t *loggingRelayTracer) RelayStatus(enabled bool) {
	log.Printf("[relay] service enabled=%v", enabled)
}

func (t *loggingRelayTracer) ConnectionOpened() {
	log.Printf("[relay] relayed connection opened")
}

func (t *loggingRelayTracer) ConnectionClosed(d time.Duration) {
	log.Printf("[relay] relayed connection closed duration=%s", d)
}

func (t *loggingRelayTracer) ConnectionRequestHandled(status pbv2.Status) {
	log.Printf("[relay] connection request status=%s", status.String())
}

func (t *loggingRelayTracer) ReservationAllowed(isRenewal bool) {
	log.Printf("[relay] reservation allowed renewal=%v", isRenewal)
}

func (t *loggingRelayTracer) ReservationClosed(cnt int) {
	log.Printf("[relay] reservation closed count=%d", cnt)
}

func (t *loggingRelayTracer) ReservationRequestHandled(status pbv2.Status) {
	log.Printf("[relay] reservation request status=%s", status.String())
}

func (t *loggingRelayTracer) BytesTransferred(cnt int) {
	log.Printf("[relay] relayed bytes=%d", cnt)
}

