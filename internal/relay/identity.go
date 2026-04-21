package relay

import (
	"log"
	"path/filepath"

	kmnet "github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// defaultIdentityFile is used when no path is set (current working directory).
const defaultIdentityFile = "relay-identity.key"

// LoadOrCreateIdentity loads an Ed25519 private key from path, or generates one and saves it.
// The file is created with mode 0600; parent directories with 0700. Same path ⇒ same peer ID across restarts.
func LoadOrCreateIdentity(path string) (crypto.PrivKey, error) {
	if path == "" {
		path = defaultIdentityFile
	}
	path = filepath.Clean(path)

	priv, created, err := kmnet.LoadOrCreateIdentityFile(path)
	if err != nil {
		return nil, err
	}
	if created {
		log.Printf("[relay] identity generated and saved to %s", path)
	} else {
		log.Printf("[relay] identity loaded from %s", path)
	}
	return priv, nil
}
