package network

import (
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestAccountP2PIdentityPath_Deterministic(t *testing.T) {
	uid := "550e8400-e29b-41d4-a716-446655440000"
	p1, err := AccountP2PIdentityPath(AccountRoleBuyer, "https://Control.EXAMPLE/v1/", uid)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := AccountP2PIdentityPath(AccountRoleBuyer, "https://control.example/v1", "550E8400-E29B-41D4-A716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatalf("expected same path, got %q vs %q", p1, p2)
	}
}

func TestAccountP2PIdentityPath_BuyerSellerDiffer(t *testing.T) {
	uid := "550e8400-e29b-41d4-a716-446655440001"
	b, err := AccountP2PIdentityPath(AccountRoleBuyer, "http://127.0.0.1:8090", uid)
	if err != nil {
		t.Fatal(err)
	}
	s, err := AccountP2PIdentityPath(AccountRoleSeller, "http://127.0.0.1:8090", uid)
	if err != nil {
		t.Fatal(err)
	}
	if b == s {
		t.Fatal("buyer and seller paths should differ for same user id")
	}
}

func TestLoadOrCreateAccountP2PIdentity_StablePeerID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "explicit.key")
	uid := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	k1, p1, err := LoadOrCreateAccountP2PIdentity(AccountRoleBuyer, "http://127.0.0.1:8090", uid, path)
	if err != nil || p1 != path {
		t.Fatalf("first: err=%v path=%q want %q", err, p1, path)
	}
	id1, err := peer.IDFromPrivateKey(k1)
	if err != nil {
		t.Fatal(err)
	}

	k2, p2, err := LoadOrCreateAccountP2PIdentity(AccountRoleBuyer, "http://127.0.0.1:8090", uid, path)
	if err != nil || p2 != path {
		t.Fatalf("second: err=%v path=%q", err, p2)
	}
	id2, err := peer.IDFromPrivateKey(k2)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("peer id changed: %s vs %s", id1, id2)
	}
}
