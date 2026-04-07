package network_test

import (
	"context"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
)

func TestDefaultHolePunchManagerConfig(t *testing.T) {
	t.Parallel()

	cfg := network.DefaultHolePunchManagerConfig()
	if cfg.RetryIntervalMin != 10*time.Second {
		t.Fatalf("RetryIntervalMin = %v, want 10s", cfg.RetryIntervalMin)
	}
	if cfg.RetryIntervalMax != 20*time.Second {
		t.Fatalf("RetryIntervalMax = %v, want 20s", cfg.RetryIntervalMax)
	}
	if cfg.MinGapBetweenAttempts != 2*time.Second {
		t.Fatalf("MinGapBetweenAttempts = %v, want 2s", cfg.MinGapBetweenAttempts)
	}
}

func TestHolePunchMetricsSuccessRate(t *testing.T) {
	t.Parallel()

	var m network.HolePunchMetrics
	if got := m.SuccessRate(); got != 0 {
		t.Fatalf("success rate with zero attempts = %v, want 0", got)
	}

	m.Attempts.Add(4)
	m.Successes.Add(3)
	if got := m.SuccessRate(); got != 0.75 {
		t.Fatalf("success rate = %v, want 0.75", got)
	}
}

func TestNewHostWithConfigAndHolePunchReturnsManager(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, mgr, err := network.NewHostWithConfigAndHolePunch(ctx, network.DefaultHostConfig(network.DefaultQUICListenAddr))
	if err != nil {
		t.Fatalf("new host with hole punch manager: %v", err)
	}
	defer h.Close()

	if mgr == nil {
		t.Fatal("expected non-nil HolePunchManager")
	}
	if mgr.Metrics() == nil {
		t.Fatal("expected non-nil HolePunchMetrics")
	}
}
