package network

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreateIdentityFile reads an Ed25519 private key from path, or generates one and saves it.
// The path must be non-empty. The file is created with mode 0600; parent directories with 0700.
// The second return value is true when a new key was written.
func LoadOrCreateIdentityFile(path string) (crypto.PrivKey, bool, error) {
	if path == "" {
		return nil, false, fmt.Errorf("identity path is empty")
	}
	path = filepath.Clean(path)

	if st, err := os.Stat(path); err == nil {
		if st.IsDir() {
			return nil, false, fmt.Errorf("identity path %q is a directory", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false, fmt.Errorf("read identity: %w", err)
		}
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err != nil {
			return nil, false, fmt.Errorf("unmarshal identity: %w", err)
		}
		return priv, false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("stat identity path: %w", err)
	}

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, -1, rand.Reader)
	if err != nil {
		return nil, false, fmt.Errorf("generate key: %w", err)
	}
	data, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, false, fmt.Errorf("marshal key: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, false, fmt.Errorf("mkdir identity dir: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, false, fmt.Errorf("save identity: %w", err)
	}
	return priv, true, nil
}
