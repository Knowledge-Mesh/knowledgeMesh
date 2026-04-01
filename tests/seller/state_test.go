package seller_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
)

func TestRejectWhenOffDuty(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	manager := seller.NewSellerStateManager(reg)

	node, err := reg.Register(seller.RegisterInput{
		Username: "state-user",
		Email:    "state-user@example.com",
		Password: "pw",
		PeerID:   "peer-state-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if _, err := manager.TurnOffDuty(node.PeerID); err != nil {
		t.Fatalf("turn off duty failed: %v", err)
	}
	_, err = manager.CheckAndConsume(node.PeerID, 10, time.Now().UTC())
	if !errors.Is(err, seller.ErrSellerOffDuty) {
		t.Fatalf("expected ErrSellerOffDuty, got %v", err)
	}
}

func TestRejectWhenOverHourlyDailyTotalLimits(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	manager := seller.NewSellerStateManager(reg)

	node, err := reg.Register(seller.RegisterInput{
		Username: "limit-user",
		Email:    "limit-user@example.com",
		Password: "pw",
		PeerID:   "peer-limit-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = manager.SetTokenLimits(node.PeerID, 100, 200, 250)
	if err != nil {
		t.Fatalf("set token limits failed: %v", err)
	}

	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CheckAndConsume(node.PeerID, 90, now); err != nil {
		t.Fatalf("first consume failed: %v", err)
	}
	if _, err := manager.CheckAndConsume(node.PeerID, 20, now); !errors.Is(err, seller.ErrHourlyLimitExceeded) {
		t.Fatalf("expected ErrHourlyLimitExceeded, got %v", err)
	}

	// Next hour, hourly resets.
	if _, err := manager.CheckAndConsume(node.PeerID, 90, now.Add(time.Hour)); err != nil {
		t.Fatalf("second-hour consume failed: %v", err)
	}
	// Daily now: 90 + 90 = 180, this pushes to 210 > 200.
	if _, err := manager.CheckAndConsume(node.PeerID, 30, now.Add(2*time.Hour)); !errors.Is(err, seller.ErrDailyLimitExceeded) {
		t.Fatalf("expected ErrDailyLimitExceeded, got %v", err)
	}
}

func TestTrackUsageAndRejectByTotalLimit(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	manager := seller.NewSellerStateManager(reg)

	node, err := reg.Register(seller.RegisterInput{
		Username: "total-user",
		Email:    "total-user@example.com",
		Password: "pw",
		PeerID:   "peer-total-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = manager.SetTokenLimits(node.PeerID, 0, 0, 100)
	if err != nil {
		t.Fatalf("set token limits failed: %v", err)
	}

	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	updated, err := manager.CheckAndConsume(node.PeerID, 60, now)
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if updated.Usage.TotalTokens != 60 {
		t.Fatalf("total usage mismatch: got %d want 60", updated.Usage.TotalTokens)
	}

	_, err = manager.CheckAndConsume(node.PeerID, 41, now.Add(3*time.Hour))
	if !errors.Is(err, seller.ErrTotalLimitExceeded) {
		t.Fatalf("expected ErrTotalLimitExceeded, got %v", err)
	}
}
