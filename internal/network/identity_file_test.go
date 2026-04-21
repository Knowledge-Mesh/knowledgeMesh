package network

import (
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestLoadOrCreateIdentityFile_StableAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.key")

	k1, created, err := LoadOrCreateIdentityFile(path)
	if err != nil || !created {
		t.Fatalf("first load: err=%v created=%v", err, created)
	}
	id1, err := peer.IDFromPrivateKey(k1)
	if err != nil {
		t.Fatal(err)
	}

	k2, created2, err := LoadOrCreateIdentityFile(path)
	if err != nil || created2 {
		t.Fatalf("second load: err=%v created=%v", err, created2)
	}
	id2, err := peer.IDFromPrivateKey(k2)
	if err != nil {
		t.Fatal(err)
	}

	if id1 != id2 {
		t.Fatalf("peer id changed after reload: %s vs %s", id1, id2)
	}
}
