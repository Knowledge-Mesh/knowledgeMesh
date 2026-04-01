package buyer_test

import (
	"errors"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
)

func createLoggedInBuyer(t *testing.T) (*buyer.Manager, buyer.State) {
	t.Helper()
	m := buyer.NewManager()
	if _, err := m.Register("b1@example.com", "b1", "pw1"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	state, err := m.Login("b1@example.com", "pw1")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	return m, state
}

func TestLimitChecks(t *testing.T) {
	t.Parallel()
	m, state := createLoggedInBuyer(t)

	_, err := m.SetLimits(state.SessionID, 100, 200, 250)
	if err != nil {
		t.Fatalf("set limits failed: %v", err)
	}
	now := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

	if _, err := m.ConsumeUsage(state.SessionID, "r1", "p1", 90, 0.2, now); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if err := m.CheckLimits(state.SessionID, 20, now); !errors.Is(err, buyer.ErrHourlyLimitExceeded) {
		t.Fatalf("expected hourly limit error, got %v", err)
	}

	if _, err := m.ConsumeUsage(state.SessionID, "r2", "p2", 90, 0.2, now.Add(time.Hour)); err != nil {
		t.Fatalf("consume next hour failed: %v", err)
	}
	if err := m.CheckLimits(state.SessionID, 30, now.Add(2*time.Hour)); !errors.Is(err, buyer.ErrDailyLimitExceeded) {
		t.Fatalf("expected daily limit error, got %v", err)
	}
}

func TestUsageUpdatesAndPromptSubmission(t *testing.T) {
	t.Parallel()
	m, state := createLoggedInBuyer(t)
	_, _ = m.SetLimits(state.SessionID, 0, 0, 100)

	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	record, err := m.SubmitPrompt(state.SessionID, "req-1", "hello", 40, 0.5, now)
	if err != nil {
		t.Fatalf("submit prompt failed: %v", err)
	}
	if record.RequestID != "req-1" {
		t.Fatalf("request id mismatch: got %q", record.RequestID)
	}

	updated, err := m.ConsumeUsage(state.SessionID, "req-2", "next", 20, 0.2, now)
	if err != nil {
		t.Fatalf("second consume failed: %v", err)
	}
	if updated.Usage.TotalTokens != 60 || updated.Usage.RequestsServed != 2 {
		t.Fatalf("usage mismatch: %+v", updated.Usage)
	}
	if len(updated.RequestHistory) != 2 {
		t.Fatalf("request history mismatch: got %d", len(updated.RequestHistory))
	}
}

func TestResetBehavior(t *testing.T) {
	t.Parallel()
	m, state := createLoggedInBuyer(t)
	now := time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC)

	_, _ = m.SetLimits(state.SessionID, 0, 0, 0)
	updated, err := m.ConsumeUsage(state.SessionID, "r1", "x", 55, 0.1, now)
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if updated.Usage.HourlyTokens != 55 || updated.Usage.DailyTokens != 55 {
		t.Fatalf("usage mismatch before reset: %+v", updated.Usage)
	}

	updated, err = m.ResetHourly(state.SessionID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("reset hourly failed: %v", err)
	}
	if updated.Usage.HourlyTokens != 0 {
		t.Fatalf("expected hourly reset to 0, got %d", updated.Usage.HourlyTokens)
	}

	updated, err = m.ResetDaily(state.SessionID, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("reset daily failed: %v", err)
	}
	if updated.Usage.DailyTokens != 0 {
		t.Fatalf("expected daily reset to 0, got %d", updated.Usage.DailyTokens)
	}
}

func TestPreferenceUpdates(t *testing.T) {
	t.Parallel()
	m, state := createLoggedInBuyer(t)

	updated, err := m.UpdatePreferences(state.SessionID, buyer.Preferences{
		PreferredSkill:     "summarization",
		PreferredModel:     "gpt-mini",
		MaxPricePerToken:   0.002,
		MaxPricePerRequest: 1.5,
	})
	if err != nil {
		t.Fatalf("update preferences failed: %v", err)
	}
	if updated.Preferences.PreferredModel != "gpt-mini" {
		t.Fatalf("preferred model mismatch: got %q", updated.Preferences.PreferredModel)
	}
	if updated.Preferences.MaxPricePerRequest != 1.5 {
		t.Fatalf("max price mismatch: got %v", updated.Preferences.MaxPricePerRequest)
	}
}
