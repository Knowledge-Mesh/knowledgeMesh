package network

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// Well-known env: explicit identity file path (overrides account-based default path).
const EnvP2PIdentityFile = "KM_P2P_IDENTITY_FILE"

// Account roles for separate key files per control account + role (buyer vs seller).
const (
	AccountRoleBuyer  = "buyer"
	AccountRoleSeller = "seller"
)

// AccountP2PIdentityPath returns the default on-disk path for a persisted libp2p identity
// for the given control pane URL and account id (buyerId or sellerId from the control API).
// Same inputs always map to the same path (password is intentionally excluded so password
// changes do not rotate the peer ID).
func AccountP2PIdentityPath(role, controlURL, userID string) (string, error) {
	role = strings.TrimSpace(role)
	if role != AccountRoleBuyer && role != AccountRoleSeller {
		return "", fmt.Errorf("identity role must be %q or %q", AccountRoleBuyer, AccountRoleSeller)
	}
	if strings.TrimSpace(controlURL) == "" || strings.TrimSpace(userID) == "" {
		return "", fmt.Errorf("control URL and user id are required for account identity path")
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	normURL := normalizeControlURL(controlURL)
	normUser := strings.ToLower(strings.TrimSpace(userID))
	sum := sha256.Sum256([]byte(role + "\n" + normURL + "\n" + normUser))
	name := fmt.Sprintf("%s-%x.key", role, sum[:16])
	return filepath.Join(base, "knowledgemesh", "p2p-identity", name), nil
}

// LoadOrCreateAccountP2PIdentity loads or creates an Ed25519 key for the account described by
// role, controlURL, and userID (buyer or seller id from the control pane). explicitPath overrides
// KM_P2P_IDENTITY_FILE and the default account-based path when non-empty.
func LoadOrCreateAccountP2PIdentity(role, controlURL, userID string, explicitPath string) (crypto.PrivKey, string, error) {
	path := strings.TrimSpace(explicitPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv(EnvP2PIdentityFile))
	}
	if path == "" {
		var err error
		path, err = AccountP2PIdentityPath(role, controlURL, userID)
		if err != nil {
			return nil, "", err
		}
	} else {
		path = filepath.Clean(path)
	}
	priv, _, err := LoadOrCreateIdentityFile(path)
	if err != nil {
		return nil, "", err
	}
	return priv, path, nil
}

func normalizeControlURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "/")
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(s)
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	path := strings.TrimSuffix(u.Path, "/")

	var b strings.Builder
	b.WriteString(scheme)
	b.WriteString("://")
	b.WriteString(host)
	def := (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
	if port != "" && !def {
		b.WriteString(":")
		b.WriteString(port)
	}
	if path != "" {
		b.WriteString(path)
	}
	return b.String()
}
