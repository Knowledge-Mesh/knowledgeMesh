package matchmaker_test

import (
	"errors"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestNodePinning_ExactMatch(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:         types.Skill{Name: "chat"},
		PreferredNode: "alice",
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", Name: "bob", OnDuty: true, Price: 0.1, Skills: []types.Skill{{Name: "chat"}}},
		{PeerID: "s2", Name: "alice", OnDuty: true, Price: 0.5, Skills: []types.Skill{{Name: "chat"}}},
		{PeerID: "s3", Name: "carol", OnDuty: true, Price: 0.2, Skills: []types.Skill{{Name: "chat"}}},
	}

	got, err := svc.Match(req, sellers)
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	// Should pick alice even though bob is cheaper
	if got.Name != "alice" {
		t.Fatalf("expected alice, got %q", got.Name)
	}
}

func TestNodePinning_CaseInsensitive(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:         types.Skill{Name: "chat"},
		PreferredNode: "Alice",
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", Name: "alice", OnDuty: true, Price: 0.5, Skills: []types.Skill{{Name: "chat"}}},
	}

	got, err := svc.Match(req, sellers)
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	if got.Name != "alice" {
		t.Fatalf("expected alice, got %q", got.Name)
	}
}

func TestNodePinning_NotFound(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:         types.Skill{Name: "chat"},
		PreferredNode: "dave",
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", Name: "alice", OnDuty: true, Price: 0.5, Skills: []types.Skill{{Name: "chat"}}},
		{PeerID: "s2", Name: "bob", OnDuty: true, Price: 0.3, Skills: []types.Skill{{Name: "chat"}}},
	}

	_, err := svc.Match(req, sellers)
	if !errors.Is(err, matchmaker.ErrNoSellerMatch) {
		t.Fatalf("expected ErrNoSellerMatch for pinned node not found, got %v", err)
	}
}

func TestNodePinning_OffDutyRejected(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:         types.Skill{Name: "chat"},
		PreferredNode: "alice",
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", Name: "alice", OnDuty: false, Price: 0.5, Skills: []types.Skill{{Name: "chat"}}},
	}

	_, err := svc.Match(req, sellers)
	if !errors.Is(err, matchmaker.ErrNoSellerMatch) {
		t.Fatalf("expected ErrNoSellerMatch for off-duty pinned node, got %v", err)
	}
}

func TestNodePinning_EmptyStringIgnored(t *testing.T) {
	t.Parallel()

	svc := matchmaker.NewService()
	req := types.InferenceRequest{
		Skill:         types.Skill{Name: "chat"},
		PreferredNode: "",
	}
	sellers := []types.SellerNode{
		{PeerID: "s1", Name: "bob", OnDuty: true, Price: 0.5, Skills: []types.Skill{{Name: "chat"}}},
		{PeerID: "s2", Name: "alice", OnDuty: true, Price: 0.2, Skills: []types.Skill{{Name: "chat"}}},
	}

	got, err := svc.Match(req, sellers)
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	// Empty preferred node = normal cheapest-first matching
	if got.Name != "alice" {
		t.Fatalf("expected cheapest seller alice, got %q", got.Name)
	}
}
