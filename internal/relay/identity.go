package relay

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

	if st, err := os.Stat(path); err == nil {
		if st.IsDir() {
			return nil, fmt.Errorf("identity path %q is a directory", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read identity: %w", err)
		}
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal identity: %w", err)
		}
		log.Printf("[relay] identity loaded from %s", path)
		return priv, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat identity path: %w", err)
	}

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, -1, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	data, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("mkdir identity dir: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}
	log.Printf("[relay] identity generated and saved to %s", path)
	return priv, nil
}
