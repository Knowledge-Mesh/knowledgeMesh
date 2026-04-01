package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

// PersistentState holds invite codes, node secrets, node configs, and user emails.
// Saved to a JSON file alongside the ledger so it survives restarts.
type PersistentState struct {
	mu          sync.RWMutex
	filepath    string
	InviteCodes map[string]bool        `json:"invite_codes"`
	NodeSecrets map[string]string      `json:"node_secrets"`
	NodeConfigs map[string]*NodeConfig `json:"node_configs"`
	UserEmails  map[string]string      `json:"user_emails"`       // name → hashed email
	ResetTokens map[string]*ResetToken `json:"reset_tokens"`      // token → reset info

	// Reverse index: secret → name. Built on load, updated on writes.
	// Not persisted — derived from NodeSecrets.
	secretIndex map[string]string `json:"-"`
}

type ResetToken struct {
	Name      string `json:"name"`
	ExpiresAt int64  `json:"expires_at"` // unix timestamp
}

func NewPersistentState(filepath string) *PersistentState {
	ps := &PersistentState{
		filepath:    filepath,
		InviteCodes: make(map[string]bool),
		NodeSecrets: make(map[string]string),
		NodeConfigs: make(map[string]*NodeConfig),
		UserEmails:  make(map[string]string),
		ResetTokens: make(map[string]*ResetToken),
		secretIndex: make(map[string]string),
	}
	ps.load()
	ps.rebuildSecretIndex()
	return ps
}

func (ps *PersistentState) load() {
	data, err := os.ReadFile(ps.filepath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[state] Warning: failed to read state file: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, ps); err != nil {
		log.Printf("[state] Warning: failed to parse state file: %v", err)
		return
	}

	// Ensure maps are initialized even if file had nulls
	if ps.InviteCodes == nil {
		ps.InviteCodes = make(map[string]bool)
	}
	if ps.NodeSecrets == nil {
		ps.NodeSecrets = make(map[string]string)
	}
	if ps.NodeConfigs == nil {
		ps.NodeConfigs = make(map[string]*NodeConfig)
	}
	if ps.UserEmails == nil {
		ps.UserEmails = make(map[string]string)
	}
	if ps.ResetTokens == nil {
		ps.ResetTokens = make(map[string]*ResetToken)
	}

	log.Printf("[state] Loaded state: %d invite codes, %d node secrets, %d node configs, %d emails",
		len(ps.InviteCodes), len(ps.NodeSecrets), len(ps.NodeConfigs), len(ps.UserEmails))
}

func (ps *PersistentState) Save() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.saveUnlocked()
}

func (ps *PersistentState) saveUnlocked() {
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		log.Printf("[state] Warning: failed to marshal state: %v", err)
		return
	}

	if err := os.WriteFile(ps.filepath, data, 0600); err != nil {
		log.Printf("[state] Warning: failed to write state file: %v", err)
	}
}

// ── Thread-safe accessors ────────────────────────────────────────────

// rebuildSecretIndex builds the reverse secret→name map from NodeSecrets.
// Must be called after load() and after any mutation to NodeSecrets.
// Caller must NOT hold the lock (this is called outside the lock).
func (ps *PersistentState) rebuildSecretIndex() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.secretIndex = make(map[string]string, len(ps.NodeSecrets))
	for name, secret := range ps.NodeSecrets {
		ps.secretIndex[secret] = name
	}
}

// rebuildSecretIndexUnlocked rebuilds the index while the lock is already held.
func (ps *PersistentState) rebuildSecretIndexUnlocked() {
	ps.secretIndex = make(map[string]string, len(ps.NodeSecrets))
	for name, secret := range ps.NodeSecrets {
		ps.secretIndex[secret] = name
	}
}

// NodeSecrets accessors

func (ps *PersistentState) GetNodeSecret(name string) (string, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	secret, ok := ps.NodeSecrets[name]
	return secret, ok
}

// IsValidNodeSecret returns true if the given secret matches any registered node.
// Uses the reverse index for O(1) lookup.
func (ps *PersistentState) IsValidNodeSecret(secret string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	_, ok := ps.secretIndex[secret]
	return ok
}

func (ps *PersistentState) SetNodeSecret(name, secret string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.NodeSecrets[name] = secret
	ps.secretIndex[secret] = name
	ps.saveUnlocked()
}

func (ps *PersistentState) DeleteNodeSecret(name string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if secret, ok := ps.NodeSecrets[name]; ok {
		delete(ps.secretIndex, secret)
	}
	delete(ps.NodeSecrets, name)
	ps.saveUnlocked()
}

// SecretForName does a reverse lookup: given a secret, returns the node name.
// Uses the reverse index for O(1) lookup instead of O(n) scan.
func (ps *PersistentState) SecretForName(secret string) string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.secretIndex[secret]
}

// InviteCodes accessors

func (ps *PersistentState) GetInviteCode(code string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.InviteCodes[code]
}

func (ps *PersistentState) SetInviteCode(code string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.InviteCodes[code] = true
	ps.saveUnlocked()
}

func (ps *PersistentState) DeleteInviteCode(code string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.InviteCodes, code)
	ps.saveUnlocked()
}

func (ps *PersistentState) InviteCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.InviteCodes)
}

func (ps *PersistentState) AllInviteCodes() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	codes := make([]string, 0, len(ps.InviteCodes))
	for code := range ps.InviteCodes {
		codes = append(codes, code)
	}
	return codes
}

// NodeConfigs accessors

func (ps *PersistentState) GetNodeConfig(name string) (NodeConfig, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	cfg, ok := ps.NodeConfigs[name]
	if !ok {
		return NodeConfig{}, false
	}
	return *cfg, true // return a copy
}

// UpdateNodeConfig applies a mutation function to a node config under the write lock.
func (ps *PersistentState) UpdateNodeConfig(name string, fn func(*NodeConfig)) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	cfg, ok := ps.NodeConfigs[name]
	if !ok {
		return false
	}
	fn(cfg)
	ps.saveUnlocked()
	return ok
}

func (ps *PersistentState) SetNodeConfig(name string, config *NodeConfig) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.NodeConfigs[name] = config
	ps.saveUnlocked()
}

func (ps *PersistentState) DeleteNodeConfig(name string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.NodeConfigs, name)
	ps.saveUnlocked()
}

// UserEmails accessors

func (ps *PersistentState) GetUserEmail(name string) (string, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	email, ok := ps.UserEmails[name]
	return email, ok
}

func (ps *PersistentState) SetUserEmail(name, emailHash string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.UserEmails[name] = emailHash
	ps.saveUnlocked()
}

func (ps *PersistentState) DeleteUserEmail(name string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.UserEmails, name)
	ps.saveUnlocked()
}

// ResetTokens accessors

func (ps *PersistentState) GetResetToken(token string) (*ResetToken, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	rt, ok := ps.ResetTokens[token]
	return rt, ok
}

func (ps *PersistentState) SetResetToken(token string, rt *ResetToken) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.ResetTokens[token] = rt
	ps.saveUnlocked()
}

func (ps *PersistentState) DeleteResetToken(token string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.ResetTokens, token)
	ps.saveUnlocked()
}

// AllResetTokens returns a copy of all pending reset tokens.
func (ps *PersistentState) AllResetTokens() map[string]*ResetToken {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	result := make(map[string]*ResetToken, len(ps.ResetTokens))
	for k, v := range ps.ResetTokens {
		result[k] = v
	}
	return result
}

// RegisterNode atomically creates a node's secret, config, email, and burns the invite.
func (ps *PersistentState) RegisterNode(name, secret, emailHash, inviteCode string, config *NodeConfig) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.NodeSecrets[name] = secret
	ps.secretIndex[secret] = name
	ps.NodeConfigs[name] = config
	ps.UserEmails[name] = emailHash
	delete(ps.InviteCodes, inviteCode)
	ps.saveUnlocked()
}

// Reset clears all maps under the lock and saves.
func (ps *PersistentState) Reset() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.InviteCodes = make(map[string]bool)
	ps.NodeSecrets = make(map[string]string)
	ps.NodeConfigs = make(map[string]*NodeConfig)
	ps.UserEmails = make(map[string]string)
	ps.ResetTokens = make(map[string]*ResetToken)
	ps.secretIndex = make(map[string]string)
	ps.saveUnlocked()
}
