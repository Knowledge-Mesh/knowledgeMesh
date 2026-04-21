package relay

import (
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestLoadOrCreateIdentity_StableAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relay.key")

	k1, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	id1, err := peer.IDFromPrivateKey(k1)
	if err != nil {
		t.Fatal(err)
	}

	k2, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := peer.IDFromPrivateKey(k2)
	if err != nil {
		t.Fatal(err)
	}

	if id1 != id2 {
		t.Fatalf("peer id changed after reload: %s vs %s", id1, id2)
	}
}
