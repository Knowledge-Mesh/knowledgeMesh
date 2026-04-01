package matchmaker_test

import (
	"errors"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestMatchExactSkill(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:    types.Skill{Name: "summarize"},
		MaxPrice: 1.0,
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", OnDuty: true, Price: 0.6, Reputation: 4.2, Skills: []types.Skill{{Name: "translate"}}},
		{PeerID: "s2", OnDuty: true, Price: 0.5, Reputation: 4.9, Skills: []types.Skill{{Name: "summarize"}}},
	}

	got, err := svc.Match(req, sellers)
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	if got.PeerID != "s2" {
		t.Fatalf("expected s2, got %q", got.PeerID)
	}
}

func TestMatchPriceFilterAndSorting(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:    types.Skill{Name: "qa"},
		MaxPrice: 0.4,
	}
	sellers := []types.SellerNode{
		{PeerID: "high", OnDuty: true, Price: 0.9, Reputation: 5.0, Skills: []types.Skill{{Name: "qa"}}},
		{PeerID: "p2", OnDuty: true, Price: 0.3, Reputation: 4.1, Skills: []types.Skill{{Name: "qa"}}},
		{PeerID: "p1", OnDuty: true, Price: 0.2, Reputation: 3.8, Skills: []types.Skill{{Name: "qa"}}},
	}

	got, err := svc.Match(req, sellers)
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	if got.PeerID != "p1" {
		t.Fatalf("expected lowest-price seller p1, got %q", got.PeerID)
	}
}

func TestNoMatchFound(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:    types.Skill{Name: "vision"},
		MaxPrice: 0.1,
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", OnDuty: false, Price: 0.05, Skills: []types.Skill{{Name: "vision"}}},
		{PeerID: "s2", OnDuty: true, Price: 0.2, Skills: []types.Skill{{Name: "vision"}}},
		{PeerID: "s3", OnDuty: true, Price: 0.05, Skills: []types.Skill{{Name: "qa"}}},
	}

	_, err := svc.Match(req, sellers)
	if !errors.Is(err, matchmaker.ErrNoSellerMatch) {
		t.Fatalf("expected ErrNoSellerMatch, got %v", err)
	}
}
