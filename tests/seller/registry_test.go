package seller_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/seller"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func TestRegisterAndLoginSeller(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))

	registered, err := reg.Register(seller.RegisterInput{
		Username:   "alice",
		Email:      "alice@example.com",
		Password:   "secret123",
		PeerID:     "12D3KooWAlice",
		Skills:     []types.Skill{{Name: "summarize", ModelName: "mini", ModelType: "llm", TuningTier: "base", Price: 0.01}},
		ModelName:  "mini",
		ModelType:  "llm",
		TuningTier: "base",
		Price:      0.01,
		ResourceHints: types.ResourceHints{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     1,
		},
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if registered.ModelName != "mini" {
		t.Fatalf("model name mismatch: got %q", registered.ModelName)
	}
	if registered.ResourceHints.CPUCores != 4 {
		t.Fatalf("cpu cores mismatch: got %d", registered.ResourceHints.CPUCores)
	}

	loggedIn, err := reg.Login(seller.LoginInput{
		UsernameOrEmail: "alice@example.com",
		Password:        "secret123",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if loggedIn.PeerID != "12D3KooWAlice" {
		t.Fatalf("peer id mismatch: got %q", loggedIn.PeerID)
	}
	if len(loggedIn.Skills) != 1 || loggedIn.Skills[0].Name != "summarize" {
		t.Fatalf("skills mismatch: got %+v", loggedIn.Skills)
	}
}

func TestLoginWithWrongPassword(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	_, err := reg.Register(seller.RegisterInput{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = reg.Login(seller.LoginInput{
		UsernameOrEmail: "bob",
		Password:        "wrong-password",
	})
	if !errors.Is(err, seller.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestDuplicateRegistrationFails(t *testing.T) {
	t.Parallel()

	reg := seller.NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	_, err := reg.Register(seller.RegisterInput{
		Username: "charlie",
		Email:    "charlie@example.com",
		Password: "pw1",
	})
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = reg.Register(seller.RegisterInput{
		Username: "charlie",
		Email:    "charlie2@example.com",
		Password: "pw2",
	})
	if !errors.Is(err, seller.ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}
}
